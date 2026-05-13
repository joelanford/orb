package cmd

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"
	"charm.land/lipgloss/v2/table"
	mmsemver "github.com/Masterminds/semver/v3"
	bsemver "github.com/blang/semver/v4"
	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
	"github.com/joelanford/library-olm/catalog/v1/fbc"
	"github.com/joelanford/library-olm/catalog/v1/sqlite"
	"github.com/joelanford/library-olm/image"
	imagecatalog "github.com/joelanford/library-olm/image/catalog"
	resolverv1 "github.com/joelanford/library-olm/resolver/v1"
	ocispecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/spf13/cobra"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	dockerTransport "go.podman.io/image/v5/docker"
	"go.podman.io/image/v5/types"
	"golang.org/x/sync/errgroup"
	"golang.org/x/term"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/util/jsonpath"
	"sigs.k8s.io/yaml"

	"github.com/joelanford/orb/internal/catalog"
	orbimage "github.com/joelanford/orb/internal/image"
	"github.com/joelanford/orb/internal/termimage"
	"github.com/joelanford/orb/internal/transport"
)

func newCatalogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "catalog",
		Short: "Manage local FBC catalogs",
	}

	cmd.AddCommand(newCatalogAddCmd())
	cmd.AddCommand(newCatalogEditCmd())
	cmd.AddCommand(newCatalogInfoCmd())
	cmd.AddCommand(newCatalogListCmd())
	cmd.AddCommand(newCatalogRemoveCmd())
	cmd.AddCommand(newCatalogResolveCmd())
	cmd.AddCommand(newCatalogSearchCmd())
	cmd.AddCommand(newCatalogUpdateCmd())

	return cmd
}

func openStore() (catalogv1.Store, error) {
	path, err := catalog.DefaultDBPath()
	if err != nil {
		return nil, err
	}
	return sqlite.OpenStore(path)
}

func newFBCImporter(fsys fs.FS) *fbc.Importer {
	return fbc.NewImporter(fsys, fbc.WithOLMPackageExtension(catalog.DisplayMetadataExtension{}))
}

type catalogAddOptions struct {
	priority int
	labels   map[string]string
}

func newCatalogAddCmd() *cobra.Command {
	opts := &catalogAddOptions{}

	cmd := &cobra.Command{
		Use:   "add NAME REF",
		Short: "Add a catalog from an OCI image",
		Long: `Add a catalog by pulling FBC content from an OCI image.

REF is a skopeo-style transport:ref string. Currently only docker:// is supported.

Examples:
  orb catalog add operatorhubio docker://quay.io/operatorhubio/catalog:latest
  orb catalog add operatorhubio docker://quay.io/operatorhubio/catalog:latest --priority 100
  orb catalog add operatorhubio docker://quay.io/operatorhubio/catalog:latest --label env=prod --label tier=community`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCatalogAdd(cmd, args[0], args[1], opts)
		},
	}

	cmd.Flags().IntVar(&opts.priority, "priority", 0, "Priority for catalog ordering (higher is preferred)")
	cmd.Flags().StringToStringVar(&opts.labels, "label", nil, "Label to set on catalog (key=value, repeatable)")

	return cmd
}

type catalogEditOptions struct {
	priority     int
	labels       map[string]string
	removeLabels []string
}

func newCatalogEditCmd() *cobra.Command {
	opts := &catalogEditOptions{}

	cmd := &cobra.Command{
		Use:   "edit NAME",
		Short: "Edit catalog settings",
		Long: `Edit settings for an existing catalog.

Examples:
  orb catalog edit operatorhubio --priority 100
  orb catalog edit operatorhubio --label env=staging
  orb catalog edit operatorhubio --remove-label tier`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCatalogEdit(cmd, args[0], opts)
		},
	}

	cmd.Flags().IntVar(&opts.priority, "priority", 0, "Priority for catalog ordering (higher is preferred)")
	cmd.Flags().StringToStringVar(&opts.labels, "label", nil, "Label to set on catalog (key=value, repeatable)")
	cmd.Flags().StringArrayVar(&opts.removeLabels, "remove-label", nil, "Label key to remove from catalog (repeatable)")

	return cmd
}

func runCatalogEdit(cmd *cobra.Command, name string, opts *catalogEditOptions) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	cat, err := store.Get(name)
	if err != nil {
		return err
	}

	var setOpts []catalogv1.SetOption
	if cmd.Flags().Changed("priority") {
		setOpts = append(setOpts, catalogv1.WithPriority(opts.priority))
	}

	if cmd.Flags().Changed("label") || len(opts.removeLabels) > 0 {
		newLabels := cat.Labels()
		if newLabels == nil {
			newLabels = make(map[string]string)
		}
		for k, v := range opts.labels {
			newLabels[k] = v
		}
		for _, k := range opts.removeLabels {
			delete(newLabels, k)
		}
		setOpts = append(setOpts, catalogv1.WithLabels(newLabels))
	}

	if len(setOpts) > 0 {
		if _, err := store.Set(cmd.Context(), name, setOpts...); err != nil {
			return err
		}
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Edited catalog %q\n", name)
	return nil
}

type catalogInfoOptions struct {
	noIcon bool
}

func newCatalogInfoCmd() *cobra.Command {
	opts := &catalogInfoOptions{}

	cmd := &cobra.Command{
		Use:   "info PACKAGE",
		Short: "Show details for a package",
		Long: `Display detailed information about a package from the highest-priority catalog that contains it.

Examples:
  orb catalog info vault
  orb catalog info cert-manager
  orb catalog info --no-icon vault`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCatalogInfo(cmd, args[0], opts)
		},
	}

	cmd.Flags().BoolVar(&opts.noIcon, "no-icon", false, "Suppress icon display")

	return cmd
}

func runCatalogInfo(cmd *cobra.Command, packageName string, opts *catalogInfoOptions) error {
	ctx := cmd.Context()

	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	catalogs, err := sortedCatalogs(store)
	if err != nil {
		return err
	}

	for _, cat := range catalogs {
		pkg, err := cat.GetPackage(ctx, packageName)
		if err != nil {
			continue
		}

		out := cmd.OutOrStdout()

		if !opts.noIcon {
			if iconJSON, err := pkg.Property(ctx, catalog.PropertyIcon); err == nil && iconJSON != nil {
				var icon catalog.IconValue
				if json.Unmarshal(iconJSON, &icon) == nil && len(icon.Data) > 0 {
					if f, ok := out.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
						if err := termimage.Render(out, icon.Data, icon.MediaType, 80); err == nil {
							fmt.Fprintln(out)
						}
					}
				}
			}
		}

		bold := lipgloss.NewStyle().Bold(true)
		fmt.Fprintf(out, "%s %s\n", bold.Render("Package:"), packageName)

		if val, err := pkg.Property(ctx, catalog.PropertyDisplayName); err == nil && val != nil {
			var displayName string
			if json.Unmarshal(val, &displayName) == nil && displayName != "" {
				fmt.Fprintf(out, "%s %s\n", bold.Render("Display Name:"), displayName)
			}
		}

		fmt.Fprintf(out, "%s %s\n", bold.Render("Catalog:"), cat.Name())

		if val, err := pkg.Property(ctx, catalog.PropertyDescription); err == nil && val != nil {
			var description string
			if json.Unmarshal(val, &description) == nil && description != "" {
				fmt.Fprintf(out, "%s %s\n", bold.Render("Description:"), description)
			}
		}

		if val, err := pkg.Property(ctx, catalog.PropertyKeywords); err == nil && val != nil {
			var keywords []string
			if json.Unmarshal(val, &keywords) == nil && len(keywords) > 0 {
				fmt.Fprintf(out, "%s %s\n", bold.Render("Keywords:"), strings.Join(keywords, ", "))
			}
		}

		type deprecationEntry struct {
			label   string
			message string
		}
		var deprecations []deprecationEntry

		composite, ok := pkg.(catalogv1.CompositeUpdateGraph)
		if ok {
			var channelNames []string
			for ch, err := range composite.ListGraphs(ctx) {
				if err != nil {
					return err
				}
				channelNames = append(channelNames, ch.Name())
				if d, ok := ch.(catalogv1.Deprecated); ok {
					deprecations = append(deprecations, deprecationEntry{
						label:   fmt.Sprintf("Channel %s", ch.Name()),
						message: d.DeprecationMessage(),
					})
				}
			}
			if len(channelNames) > 0 {
				slices.Sort(channelNames)
				fmt.Fprintf(out, "%s %s\n", bold.Render("Channels:"), strings.Join(channelNames, ", "))
			}
		}

		var bundleCount int
		var highest bundlev1.Bundle
		for b, err := range pkg.ListBundles(ctx) {
			if err != nil {
				return err
			}
			bundleCount++
			if highest == nil || b.NameVersionRelease().VersionRelease().Compare(highest.NameVersionRelease().VersionRelease()) > 0 {
				highest = b
			}
			if d, ok := b.(catalogv1.Deprecated); ok {
				deprecations = append(deprecations, deprecationEntry{
					label:   fmt.Sprintf("Bundle %s", b.ID()),
					message: d.DeprecationMessage(),
				})
			}
		}
		if bundleCount > 0 {
			nvr := highest.NameVersionRelease()
			vr := nvr.VersionRelease()
			fmt.Fprintf(out, "%s %d (latest: %s)\n", bold.Render("Versions:"), bundleCount, formatVersionRelease(vr))
		}

		if d, ok := pkg.(catalogv1.Deprecated); ok {
			deprecations = append([]deprecationEntry{{
				label:   "Package",
				message: d.DeprecationMessage(),
			}}, deprecations...)
		}
		if len(deprecations) > 0 {
			fmt.Fprintf(out, "%s\n", bold.Render("Deprecations:"))
			for _, entry := range deprecations {
				fmt.Fprintf(out, "  %s: %s\n", entry.label, entry.message)
			}
		}

		return nil
	}

	return fmt.Errorf("package %q not found in any catalog", packageName)
}

type catalogSearchOptions struct {
	includeShadowed   bool
	includeDeprecated bool
}

func newCatalogSearchCmd() *cobra.Command {
	opts := &catalogSearchOptions{}

	cmd := &cobra.Command{
		Use:   "search [KEYWORD]",
		Short: "Search or list packages across catalogs",
		Long: `Search for packages across all catalogs by keyword, or list all packages
when no keyword is given.

The keyword is matched (case-insensitive) against the package name, display name,
description, and keyword entries. By default, only the highest-priority entry for
each package is shown and deprecated packages are hidden. Use --include-shadowed
to also show lower-priority duplicates (marked with * and dimmed), and
--include-deprecated to show deprecated packages (marked with † and dimmed).

Examples:
  orb catalog search
  orb catalog search vault
  orb catalog search security
  orb catalog search --include-shadowed
  orb catalog search --include-deprecated`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var keyword string
			if len(args) > 0 {
				keyword = args[0]
			}
			return runCatalogSearch(cmd, keyword, opts)
		},
	}

	cmd.Flags().BoolVar(&opts.includeShadowed, "include-shadowed", false, "Include lower-priority duplicates shadowed by higher-priority catalogs")
	cmd.Flags().BoolVar(&opts.includeDeprecated, "include-deprecated", false, "Include deprecated packages (marked with †)")

	return cmd
}

func runCatalogSearch(cmd *cobra.Command, keyword string, opts *catalogSearchOptions) error {
	ctx := cmd.Context()

	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	catalogs, err := sortedCatalogs(store)
	if err != nil {
		return err
	}

	type searchResult struct {
		catalogName string
		packageName string
		displayName string
		shadowed    bool
		deprecated  bool
	}

	seen := make(map[string]struct{})
	var results []searchResult
	lowerKeyword := strings.ToLower(keyword)

	for _, cat := range catalogs {
		for pkg, err := range cat.ListPackages(ctx) {
			if err != nil {
				return err
			}

			var displayName, description string
			var keywords []string

			if val, err := pkg.Property(ctx, catalog.PropertyDisplayName); err == nil && val != nil {
				_ = json.Unmarshal(val, &displayName)
			}
			if val, err := pkg.Property(ctx, catalog.PropertyDescription); err == nil && val != nil {
				_ = json.Unmarshal(val, &description)
			}
			if val, err := pkg.Property(ctx, catalog.PropertyKeywords); err == nil && val != nil {
				_ = json.Unmarshal(val, &keywords)
			}

			if keyword != "" && !matchesKeyword(lowerKeyword, pkg.Name(), displayName, description, keywords) {
				continue
			}

			_, pkgDeprecated := pkg.(catalogv1.Deprecated)
			if pkgDeprecated && !opts.includeDeprecated {
				continue
			}

			_, shadowed := seen[pkg.Name()]
			seen[pkg.Name()] = struct{}{}
			if shadowed && !opts.includeShadowed {
				continue
			}
			results = append(results, searchResult{
				catalogName: cat.Name(),
				packageName: pkg.Name(),
				displayName: displayName,
				shadowed:    shadowed,
				deprecated:  pkgDeprecated,
			})
		}
	}

	slices.SortStableFunc(results, func(a, b searchResult) int {
		return strings.Compare(a.packageName, b.packageName)
	})

	if len(results) == 0 {
		if keyword != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "No packages found matching %q.\n", keyword)
		} else {
			fmt.Fprintln(cmd.OutOrStdout(), "No packages found.")
		}
		return nil
	}

	out := cmd.OutOrStdout()

	hasShadowed := false
	hasDeprecated := false
	dimmedRows := make(map[int]bool)
	var rows [][]string
	for i, r := range results {
		pkgName := r.packageName
		catName := r.catalogName
		if r.shadowed {
			catName += "*"
			hasShadowed = true
			dimmedRows[i] = true
		}
		if r.deprecated {
			pkgName += "†"
			hasDeprecated = true
			dimmedRows[i] = true
		}
		rows = append(rows, []string{pkgName, r.displayName, catName})
	}

	baseStyle := lipgloss.NewStyle().PaddingRight(2)
	t := table.New().
		Headers("PACKAGE", "DISPLAY NAME", "CATALOG").
		Rows(rows...).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderHeader(false).
		BorderColumn(false).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row >= 0 && dimmedRows[row] {
				return baseStyle.Faint(true)
			}
			return baseStyle
		})

	fmt.Fprintln(out, t.Render())
	var legend []string
	if hasShadowed {
		legend = append(legend, "* = shadowed by a higher-priority catalog")
	}
	if hasDeprecated {
		legend = append(legend, "† = deprecated")
	}
	if len(legend) > 0 {
		fmt.Fprintf(out, "\n%s\n", strings.Join(legend, "\n"))
	}
	return nil
}

func matchesKeyword(lowerKeyword, packageName, displayName, description string, keywords []string) bool {
	if strings.Contains(strings.ToLower(packageName), lowerKeyword) {
		return true
	}
	if strings.Contains(strings.ToLower(displayName), lowerKeyword) {
		return true
	}
	if strings.Contains(strings.ToLower(description), lowerKeyword) {
		return true
	}
	for _, kw := range keywords {
		if strings.Contains(strings.ToLower(kw), lowerKeyword) {
			return true
		}
	}
	return false
}

func newCatalogListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List configured catalogs",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCatalogList(cmd)
		},
	}
}

func runCatalogList(cmd *cobra.Command) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	catalogs, err := sortedCatalogs(store)
	if err != nil {
		return err
	}

	if len(catalogs) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No catalogs configured.")
		return nil
	}

	var rows [][]string
	for _, cat := range catalogs {
		rows = append(rows, []string{cat.Name(), cat.URI(), fmt.Sprintf("%d", cat.Priority())})
	}
	t := table.New().
		Headers("NAME", "REF", "PRIORITY").
		Rows(rows...).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderHeader(false).
		BorderColumn(false).
		StyleFunc(func(row, col int) lipgloss.Style {
			return lipgloss.NewStyle().PaddingRight(2)
		})
	fmt.Fprintln(cmd.OutOrStdout(), t.Render())
	return nil
}

func newCatalogRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove NAME",
		Short: "Remove a catalog",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCatalogRemove(cmd, args[0])
		},
	}
}

type catalogUpdateOptions struct {
	force bool
}

func newCatalogUpdateCmd() *cobra.Command {
	opts := &catalogUpdateOptions{}

	cmd := &cobra.Command{
		Use:   "update [NAME]",
		Short: "Update one or all catalogs",
		Long: `Update catalogs by re-pulling FBC content from their OCI images.

If NAME is given, only that catalog is updated. Otherwise all catalogs are updated.
If the resolved digest has not changed, the pull is skipped unless --force is set.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var name string
			if len(args) > 0 {
				name = args[0]
			}
			return runCatalogUpdate(cmd, name, opts)
		},
	}

	cmd.Flags().BoolVarP(&opts.force, "force", "f", false, "Force re-pull even if the digest has not changed")

	return cmd
}

func runCatalogAdd(cmd *cobra.Command, name, ref string, opts *catalogAddOptions) error {
	ctx := cmd.Context()

	tRef, err := transport.ParseRef(ref)
	if err != nil {
		return err
	}
	if tRef.Transport != transport.Docker {
		return fmt.Errorf("only docker:// transport is supported for catalogs")
	}

	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	if _, err := store.Get(name); err == nil {
		return fmt.Errorf("catalog %q already exists", name)
	}

	repo, err := newDockerRepo(ctx, tRef.Ref)
	if err != nil {
		return err
	}
	defer repo.Close()

	handler := &imagecatalog.FBCHandler{}
	desc, manifestBytes, err := orbimage.ResolveAndMatch(ctx, repo, handler)
	if err != nil {
		return err
	}

	discoveredDescs, err := handler.Discover(ctx, repo, desc, manifestBytes)
	if err != nil {
		return fmt.Errorf("discovering image content: %w", err)
	}
	total := sumDescriptorSizes(discoveredDescs)
	alreadyFetched := sumDescriptorSizes(repo.CachedDescriptors())

	p := mpb.New(mpb.WithOutput(cmd.ErrOrStderr()))
	bar := p.AddBar(0,
		mpb.PrependDecorators(
			decor.Name(name, decor.WCSyncSpaceR),
			decor.Counters(decor.SizeB1024(0), "% .1f / % .1f", decor.WCSyncSpace),
		),
	)
	bar.SetTotal(total, false)
	bar.IncrBy(int(alreadyFetched))
	repo.SetOnRead(func(n int) { bar.IncrBy(n) })

	tmpDir, err := os.MkdirTemp("", "orb-catalog-*")
	if err != nil {
		bar.Abort(true)
		p.Wait()
		return fmt.Errorf("creating temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := handler.Unpack(ctx, repo, desc, manifestBytes, tmpDir); err != nil {
		bar.Abort(true)
		p.Wait()
		return fmt.Errorf("unpacking catalog: %w", err)
	}

	bar.SetTotal(total, true)
	p.Wait()

	importer := newFBCImporter(os.DirFS(tmpDir))
	setOpts := []catalogv1.SetOption{
		catalogv1.WithURI(ref),
		catalogv1.WithPriority(opts.priority),
		catalogv1.WithContent(importer, desc.Digest.String()),
	}
	if opts.labels != nil {
		setOpts = append(setOpts, catalogv1.WithLabels(opts.labels))
	}

	if _, err := store.Set(ctx, name, setOpts...); err != nil {
		var partial catalogv1.PartialImportError
		if !errors.As(err, &partial) {
			return err
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: partial import for catalog %q: %v\n", name, err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Added catalog %q (%s)\n", name, desc.Digest)
	return nil
}

func runCatalogRemove(cmd *cobra.Command, name string) error {
	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	if err := store.Delete(name); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Removed catalog %q\n", name)
	return nil
}

func runCatalogUpdate(cmd *cobra.Command, name string, opts *catalogUpdateOptions) error {
	ctx := cmd.Context()

	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	var toUpdate []catalogv1.Catalog
	if name != "" {
		cat, err := store.Get(name)
		if err != nil {
			return err
		}
		toUpdate = []catalogv1.Catalog{cat}
	} else {
		catalogs, err := store.List()
		if err != nil {
			return err
		}
		toUpdate = catalogs
	}

	const numWorkers = 3

	type resolvedCatalog struct {
		name     string
		uri      string
		digest   string
		upToDate bool
		repo     *orbimage.Repository
		err      error
		warning  string
	}

	resolved := make([]resolvedCatalog, len(toUpdate))

	resolveGroup, resolveCtx := errgroup.WithContext(ctx)
	resolveGroup.SetLimit(numWorkers)
	for idx, cat := range toUpdate {
		resolveGroup.Go(func() error {
			res := &resolved[idx]
			res.name = cat.Name()
			res.uri = cat.URI()

			tRef, err := transport.ParseRef(cat.URI())
			if err != nil {
				res.err = fmt.Errorf("catalog %q: %w", cat.Name(), err)
				return nil
			}

			repo, err := newDockerRepo(resolveCtx, tRef.Ref)
			if err != nil {
				res.err = fmt.Errorf("catalog %q: %w", cat.Name(), err)
				return nil
			}

			desc, err := repo.Resolve(resolveCtx)
			if err != nil {
				repo.Close()
				res.err = fmt.Errorf("catalog %q: resolving image: %w", cat.Name(), err)
				return nil
			}

			res.digest = desc.Digest.String()

			if !opts.force && desc.Digest.String() == cat.Digest() {
				res.upToDate = true
				repo.Close()
				return nil
			}

			res.repo = repo
			return nil
		})
	}
	if err := resolveGroup.Wait(); err != nil {
		for i := range resolved {
			if resolved[i].repo != nil {
				resolved[i].repo.Close()
			}
		}
		return err
	}

	p := mpb.New(mpb.WithOutput(cmd.ErrOrStderr()))

	type downloadTarget struct {
		idx int
		bar *mpb.Bar
	}
	var targets []downloadTarget
	for i := range resolved {
		res := &resolved[i]
		if res.err != nil || res.upToDate {
			continue
		}
		bar := p.AddBar(0,
			mpb.PrependDecorators(
				decor.Name(res.name, decor.WCSyncSpaceR),
				decor.Counters(decor.SizeB1024(0), "% .1f / % .1f", decor.WCSyncSpace),
			),
		)
		targets = append(targets, downloadTarget{idx: i, bar: bar})
	}

	unpackGroup, unpackCtx := errgroup.WithContext(ctx)
	unpackGroup.SetLimit(numWorkers)
	for _, t := range targets {
		unpackGroup.Go(func() error {
			res := &resolved[t.idx]
			bar := t.bar
			defer res.repo.Close()

			handler := &imagecatalog.FBCHandler{}
			desc, manifestBytes, err := orbimage.ResolveAndMatch(unpackCtx, res.repo, handler)
			if err != nil {
				res.err = fmt.Errorf("catalog %q: %w", res.name, err)
				bar.Abort(true)
				return nil
			}

			discoveredDescs, err := handler.Discover(unpackCtx, res.repo, desc, manifestBytes)
			if err != nil {
				res.err = fmt.Errorf("catalog %q: discovering image content: %w", res.name, err)
				bar.Abort(true)
				return nil
			}
			total := sumDescriptorSizes(discoveredDescs)
			alreadyFetched := sumDescriptorSizes(res.repo.CachedDescriptors())
			bar.SetTotal(total, false)
			bar.IncrBy(int(alreadyFetched))
			res.repo.SetOnRead(func(n int) { bar.IncrBy(n) })

			tmpDir, err := os.MkdirTemp("", "orb-catalog-*")
			if err != nil {
				res.err = fmt.Errorf("catalog %q: creating temp directory: %w", res.name, err)
				bar.Abort(true)
				return nil
			}
			defer os.RemoveAll(tmpDir)

			if err := handler.Unpack(unpackCtx, res.repo, desc, manifestBytes, tmpDir); err != nil {
				res.err = fmt.Errorf("catalog %q: unpacking catalog: %w", res.name, err)
				bar.Abort(true)
				return nil
			}

			importer := newFBCImporter(os.DirFS(tmpDir))
			if _, err := store.Set(unpackCtx, res.name,
				catalogv1.WithContent(importer, res.digest),
			); err != nil {
				var partial catalogv1.PartialImportError
				if !errors.As(err, &partial) {
					res.err = fmt.Errorf("catalog %q: importing content: %w", res.name, err)
					bar.Abort(true)
					return nil
				}
				res.warning = fmt.Sprintf("catalog %q: partial import: %v", res.name, err)
			}

			bar.SetTotal(total, true)
			return nil
		})
	}
	if err := unpackGroup.Wait(); err != nil {
		p.Wait()
		return err
	}
	p.Wait()

	var allErrors []error
	for i := range resolved {
		res := &resolved[i]
		if res.err != nil {
			allErrors = append(allErrors, res.err)
		}
	}

	out := cmd.OutOrStdout()
	for i := range resolved {
		res := &resolved[i]
		if res.err != nil {
			continue
		}
		if res.upToDate {
			fmt.Fprintf(out, "Catalog %q is up-to-date (%s)\n", res.name, res.digest)
		} else {
			fmt.Fprintf(out, "Catalog %q updated (%s)\n", res.name, res.digest)
		}
		if res.warning != "" {
			fmt.Fprintf(cmd.ErrOrStderr(), "Warning: %s\n", res.warning)
		}
	}

	return errors.Join(allErrors...)
}

type catalogResolveOptions struct {
	catalogLabelSelector string
	channels             []string
	version              string
	installed            string
	output               string
	includeDeprecated    bool
}

func newCatalogResolveCmd() *cobra.Command {
	opts := &catalogResolveOptions{}

	cmd := &cobra.Command{
		Use:   "resolve PACKAGE",
		Short: "Resolve matching bundles for a package",
		Long: `Resolve matching bundle versions for a package from configured catalogs.

Catalogs are searched in priority order (highest first). The first catalog
that contains the package is used for resolution. All matching bundles are
returned, sorted by version descending. Non-deprecated bundles are preferred
over deprecated ones. Deprecated bundles are hidden by default; use
--include-deprecated to show them (marked with † and dimmed in table output,
with "deprecated" and "deprecationMessage" fields in JSON/YAML output).

Output formats:
  (default)                  Table
  -o json                    JSON
  -o yaml                    YAML
  -o jsonpath=TEMPLATE       JSONPath template (applied to the result array)

Examples:
  orb catalog resolve vault
  orb catalog resolve vault --channel beta
  orb catalog resolve vault --version ">=0.4.0"
  orb catalog resolve vault -l env=prod
  orb catalog resolve vault --installed vault-operator.v0.4.10=0.4.10
  orb catalog resolve vault -o json
  orb catalog resolve vault -o yaml
  orb catalog resolve vault -o jsonpath='{.items[0].image}'
  orb catalog resolve vault --include-deprecated`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCatalogResolve(cmd, args[0], opts)
		},
	}

	cmd.Flags().StringVarP(&opts.catalogLabelSelector, "catalog-label-selector", "l", "", "Kubernetes label selector for filtering catalogs")
	cmd.Flags().StringArrayVar(&opts.channels, "channel", nil, "Channel to search (repeatable; if omitted, all channels are searched)")
	cmd.Flags().StringVar(&opts.version, "version", "", "Semver version constraint (e.g. ^1.0, >=1.0.0)")
	cmd.Flags().StringVar(&opts.installed, "installed", "", "Installed bundle name=version for successor resolution (e.g. vault-operator.v0.4.10=0.4.10)")
	cmd.Flags().StringVarP(&opts.output, "output", "o", "", "Output format: json, yaml, or jsonpath=TEMPLATE")
	cmd.Flags().BoolVar(&opts.includeDeprecated, "include-deprecated", false, "Include deprecated bundles (marked with † in table output)")

	return cmd
}

func runCatalogResolve(cmd *cobra.Command, packageName string, opts *catalogResolveOptions) error {
	ctx := cmd.Context()

	store, err := openStore()
	if err != nil {
		return err
	}
	defer store.Close()

	var reader catalogv1.StoreReader = store
	if opts.catalogLabelSelector != "" {
		selector, err := labels.Parse(opts.catalogLabelSelector)
		if err != nil {
			return fmt.Errorf("parsing catalog label selector: %w", err)
		}
		reader = store.Select(selector)
	}

	resolveOpts := []resolverv1.ResolveOption{
		resolverv1.PreferNonDeprecatedBundles(),
	}

	if len(opts.channels) > 0 {
		paths := make([][]string, len(opts.channels))
		for i, ch := range opts.channels {
			paths[i] = []string{ch}
		}
		resolveOpts = append(resolveOpts, resolverv1.WithGraphs(paths))
	}

	if opts.version != "" {
		constraint, err := mmsemver.NewConstraint(opts.version)
		if err != nil {
			return fmt.Errorf("parsing version constraint: %w", err)
		}
		resolveOpts = append(resolveOpts, resolverv1.WithMastermindsVersionConstraint(*constraint))
	}

	if opts.installed != "" {
		bundleID, versionStr, ok := strings.Cut(opts.installed, "=")
		if !ok {
			return fmt.Errorf("invalid --installed value %q: expected format name=version", opts.installed)
		}
		v, err := bsemver.Parse(versionStr)
		if err != nil {
			return fmt.Errorf("parsing installed version %q: %w", versionStr, err)
		}
		identity := installedBundleIdentity{
			id:  bundlev1.BundleID(bundleID),
			nvr: bundlev1.NameVersionRelease{Version: v},
		}
		resolveOpts = append(resolveOpts, resolverv1.WithSuccessorsOf(identity))
	}

	result, err := resolverv1.Resolve(ctx, reader, packageName, resolveOpts...)
	if err != nil {
		return err
	}
	if result == nil {
		return fmt.Errorf("package %q not found in any catalog", packageName)
	}

	return printResolveResults(cmd.OutOrStdout(), result.Catalog, result.Bundles, opts.output, opts.includeDeprecated)
}

type installedBundleIdentity struct {
	id  bundlev1.BundleID
	nvr bundlev1.NameVersionRelease
}

func (i installedBundleIdentity) ID() bundlev1.BundleID                           { return i.id }
func (i installedBundleIdentity) NameVersionRelease() bundlev1.NameVersionRelease { return i.nvr }

type resolveResultItem struct {
	Catalog            string `json:"catalog"`
	Bundle             string `json:"bundle"`
	Version            string `json:"version"`
	Image              string `json:"image"`
	Deprecated         bool   `json:"deprecated"`
	DeprecationMessage string `json:"deprecationMessage,omitempty"`
}

type resolveOutput struct {
	Items []resolveResultItem `json:"items"`
}

func printResolveResults(out io.Writer, cat catalogv1.Catalog, bundles []bundlev1.Bundle, format string, includeDeprecated bool) error {
	var items []resolveResultItem
	for _, b := range bundles {
		nvr := b.NameVersionRelease()
		item := resolveResultItem{
			Catalog: cat.Name(),
			Bundle:  string(b.ID()),
			Version: formatVersionRelease(nvr.VersionRelease()),
			Image:   b.URI(),
		}
		if d, ok := b.(catalogv1.Deprecated); ok {
			if !includeDeprecated {
				continue
			}
			item.Deprecated = true
			item.DeprecationMessage = d.DeprecationMessage()
		}
		items = append(items, item)
	}
	output := resolveOutput{Items: items}

	switch {
	case format == "json":
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(output)
	case format == "yaml":
		data, err := yaml.Marshal(output)
		if err != nil {
			return err
		}
		_, err = out.Write(data)
		return err
	case strings.HasPrefix(format, "jsonpath="):
		template := strings.TrimPrefix(format, "jsonpath=")
		jp := jsonpath.New("resolve")
		if err := jp.Parse(template); err != nil {
			return fmt.Errorf("parsing jsonpath template: %w", err)
		}
		raw, err := json.Marshal(output)
		if err != nil {
			return err
		}
		var data interface{}
		if err := json.Unmarshal(raw, &data); err != nil {
			return err
		}
		var buf bytes.Buffer
		if err := jp.Execute(&buf, data); err != nil {
			return fmt.Errorf("executing jsonpath: %w", err)
		}
		_, err = fmt.Fprintln(out, buf.String())
		return err
	case format == "":
		hasDeprecated := false
		dimmedRows := make(map[int]bool)
		var rows [][]string
		for i, item := range items {
			version := item.Version
			if item.Deprecated {
				version += "†"
				hasDeprecated = true
				dimmedRows[i] = true
			}
			rows = append(rows, []string{item.Catalog, version, item.Image})
		}
		baseStyle := lipgloss.NewStyle().PaddingRight(2)
		t := table.New().
			Headers("CATALOG", "VERSION", "IMAGE").
			Rows(rows...).
			BorderTop(false).
			BorderBottom(false).
			BorderLeft(false).
			BorderRight(false).
			BorderHeader(false).
			BorderColumn(false).
			StyleFunc(func(row, col int) lipgloss.Style {
				if row >= 0 && dimmedRows[row] {
					return baseStyle.Faint(true)
				}
				return baseStyle
			})
		fmt.Fprintln(out, t.Render())
		if hasDeprecated {
			fmt.Fprintf(out, "\n† = deprecated\n")
		}
		return nil
	default:
		return fmt.Errorf("unsupported output format %q: use json, yaml, or jsonpath=TEMPLATE", format)
	}
}

func formatVersionRelease(vr bundlev1.VersionRelease) string {
	if vr.Release.IsEmpty() {
		return vr.Version.String()
	}
	return fmt.Sprintf("%s_%s", vr.Version, vr.Release)
}

func sortedCatalogs(store catalogv1.Store) ([]catalogv1.Catalog, error) {
	catalogs, err := store.List()
	if err != nil {
		return nil, err
	}
	slices.SortFunc(catalogs, func(a, b catalogv1.Catalog) int {
		if c := cmp.Compare(b.Priority(), a.Priority()); c != 0 {
			return c
		}
		return cmp.Compare(a.Name(), b.Name())
	})
	return catalogs, nil
}

func newDockerRepo(ctx context.Context, ref string) (*orbimage.Repository, error) {
	imgRef, err := dockerTransport.ParseReference("//" + ref)
	if err != nil {
		return nil, fmt.Errorf("parsing docker reference: %w", err)
	}

	client, err := image.NewContainersImageRepository(ctx, imgRef, &types.SystemContext{})
	if err != nil {
		return nil, fmt.Errorf("creating image repository: %w", err)
	}

	repo, err := orbimage.NewRepository(client)
	if err != nil {
		client.Close()
		return nil, err
	}
	return repo, nil
}

func sumDescriptorSizes(descs []ocispecv1.Descriptor) int64 {
	var total int64
	for _, d := range descs {
		total += d.Size
	}
	return total
}
