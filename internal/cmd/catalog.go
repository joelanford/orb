package cmd

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/opencontainers/go-digest"
	"github.com/spf13/cobra"
	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"
	dockerTransport "go.podman.io/image/v5/docker"
	"go.podman.io/image/v5/types"
	"k8s.io/client-go/util/jsonpath"
	"sigs.k8s.io/yaml"

	"golang.org/x/sync/errgroup"

	"github.com/joelanford/orb/internal/catalog"
	"github.com/joelanford/orb/internal/image"
	"github.com/joelanford/orb/internal/transport"
)

func newTabWriter(out io.Writer) *tabwriter.Writer {
	return tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
}

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
	db, err := catalog.OpenDefaultDB()
	if err != nil {
		return err
	}
	defer db.Close()

	err = db.UpdateCatalog(name, func(cat *catalog.Catalog) {
		if cmd.Flags().Changed("priority") {
			cat.Priority = opts.priority
		}

		if cmd.Flags().Changed("label") {
			if cat.Labels == nil {
				cat.Labels = make(map[string]string)
			}
			for k, v := range opts.labels {
				cat.Labels[k] = v
			}
		}

		for _, k := range opts.removeLabels {
			delete(cat.Labels, k)
		}
		if len(cat.Labels) == 0 {
			cat.Labels = nil
		}
	})
	if err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Edited catalog %q\n", name)
	return nil
}

func newCatalogInfoCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "info PACKAGE",
		Short: "Show details for a package",
		Long: `Display detailed information about a package from the highest-priority catalog that contains it.

Examples:
  orb catalog info vault
  orb catalog info cert-manager`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCatalogInfo(cmd, args[0])
		},
	}
}

func runCatalogInfo(cmd *cobra.Command, packageName string) error {
	db, err := catalog.OpenDefaultDB()
	if err != nil {
		return err
	}
	defer db.Close()

	catalogs, err := db.SortedCatalogs()
	if err != nil {
		return err
	}

	for _, cat := range catalogs {
		pd, err := db.GetPackageData(cat.Name, packageName)
		if err != nil {
			return err
		}
		if pd == nil {
			continue
		}

		out := cmd.OutOrStdout()
		fmt.Fprintf(out, "Package:       %s\n", packageName)
		if pd.DisplayName != "" {
			fmt.Fprintf(out, "Display Name:  %s\n", pd.DisplayName)
		}
		fmt.Fprintf(out, "Catalog:       %s\n", cat.Name)
		if pd.Description != "" {
			fmt.Fprintf(out, "Description:   %s\n", pd.Description)
		}
		if len(pd.Keywords) > 0 {
			fmt.Fprintf(out, "Keywords:      %s\n", strings.Join(pd.Keywords, ", "))
		}
		if len(pd.Channels) > 0 {
			channelNames := make([]string, len(pd.Channels))
			for i, ch := range pd.Channels {
				channelNames[i] = ch.Name
			}
			fmt.Fprintf(out, "Channels:      %s\n", strings.Join(channelNames, ", "))
		}
		if len(pd.Bundles) > 0 {
			latest := slices.MaxFunc(pd.Bundles, catalog.CompareBundleData)
			fmt.Fprintf(out, "Versions:      %d (latest: %s)\n", len(pd.Bundles), latest.VersionRelease)
		}
		return nil
	}

	return fmt.Errorf("package %q not found in any catalog", packageName)
}

func newCatalogSearchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "search KEYWORD",
		Short: "Search packages by keyword",
		Long: `Search for packages across all catalogs by keyword.

The keyword is matched (case-insensitive) against the package name, display name,
description, and keyword entries. Results are deduplicated by package name, keeping
the match from the highest-priority catalog.

Examples:
  orb catalog search vault
  orb catalog search security
  orb catalog search certificate`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCatalogSearch(cmd, args[0])
		},
	}
}

func runCatalogSearch(cmd *cobra.Command, keyword string) error {
	db, err := catalog.OpenDefaultDB()
	if err != nil {
		return err
	}
	defer db.Close()

	type searchResult struct {
		catalogName string
		packageName string
		displayName string
	}

	seen := make(map[string]struct{})
	var results []searchResult
	lowerKeyword := strings.ToLower(keyword)

	err = db.SearchPackageData(func(catalogName, packageName string, pd *catalog.PackageData) bool {
		if _, ok := seen[packageName]; ok {
			return true
		}

		if matchesKeyword(lowerKeyword, packageName, pd) {
			seen[packageName] = struct{}{}
			results = append(results, searchResult{
				catalogName: catalogName,
				packageName: packageName,
				displayName: pd.DisplayName,
			})
		}
		return true
	})
	if err != nil {
		return err
	}

	if len(results) == 0 {
		fmt.Fprintf(cmd.OutOrStdout(), "No packages found matching %q.\n", keyword)
		return nil
	}

	w := newTabWriter(cmd.OutOrStdout())
	fmt.Fprintln(w, "CATALOG\tPACKAGE\tDISPLAY NAME")
	for _, r := range results {
		fmt.Fprintf(w, "%s\t%s\t%s\n", r.catalogName, r.packageName, r.displayName)
	}
	return w.Flush()
}

func matchesKeyword(lowerKeyword string, packageName string, pd *catalog.PackageData) bool {
	if strings.Contains(strings.ToLower(packageName), lowerKeyword) {
		return true
	}
	if strings.Contains(strings.ToLower(pd.DisplayName), lowerKeyword) {
		return true
	}
	if strings.Contains(strings.ToLower(pd.Description), lowerKeyword) {
		return true
	}
	for _, kw := range pd.Keywords {
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
	db, err := catalog.OpenDefaultDB()
	if err != nil {
		return err
	}
	defer db.Close()

	catalogs, err := db.SortedCatalogs()
	if err != nil {
		return err
	}

	if len(catalogs) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No catalogs configured.")
		return nil
	}

	w := newTabWriter(cmd.OutOrStdout())
	fmt.Fprintln(w, "NAME\tREF\tPRIORITY")
	for _, cat := range catalogs {
		fmt.Fprintf(w, "%s\t%s\t%d\n", cat.Name, cat.Ref, cat.Priority)
	}
	return w.Flush()
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

	db, err := catalog.OpenDefaultDB()
	if err != nil {
		return err
	}
	defer db.Close()

	if cat, err := db.GetCatalog(name); err != nil {
		return err
	} else if cat != nil {
		return fmt.Errorf("catalog %q already exists", name)
	}

	repo, err := newDockerCachingRepo(ctx, tRef.Ref)
	if err != nil {
		return err
	}
	defer repo.Close()

	resolver := image.NewResolver()
	resolver.Register(&image.FBCHandler{})

	total, err := resolver.TotalSize(ctx, repo)
	if err != nil {
		return fmt.Errorf("computing total size: %w", err)
	}

	// Progress bar for download feedback.
	p := mpb.New(mpb.WithOutput(cmd.ErrOrStderr()))
	bar := p.AddBar(0,
		mpb.PrependDecorators(
			decor.Name(name, decor.WCSyncSpaceR),
			decor.Counters(decor.SizeB1024(0), "% .1f / % .1f", decor.WCSyncSpace),
		),
	)
	bar.SetTotal(total, false)

	// Set callback AFTER TotalSize so config blob doesn't count toward progress.
	repo.SetOnBytesRead(func(n int) { bar.IncrBy(n) })

	desc, err := repo.Resolve(ctx)
	if err != nil {
		bar.Abort(true)
		p.Wait()
		return fmt.Errorf("resolving image: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "orb-catalog-*")
	if err != nil {
		bar.Abort(true)
		p.Wait()
		return fmt.Errorf("creating temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if err := resolver.Unpack(ctx, repo, tmpDir); err != nil {
		bar.Abort(true)
		p.Wait()
		return fmt.Errorf("unpacking catalog: %w", err)
	}

	bar.SetTotal(total, true)
	p.Wait()

	pkgData, err := catalog.BuildPackageData(ctx, os.DirFS(tmpDir))
	if err != nil {
		return fmt.Errorf("building package data: %w", err)
	}

	if err := db.AddCatalog(catalog.Catalog{
		Name:     name,
		Ref:      ref,
		Digest:   desc.Digest.String(),
		Priority: opts.priority,
		Labels:   opts.labels,
	}); err != nil {
		return err
	}

	if err := db.SetPackageData(name, pkgData); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Added catalog %q (%s)\n", name, desc.Digest)
	return nil
}

func runCatalogRemove(cmd *cobra.Command, name string) error {
	db, err := catalog.OpenDefaultDB()
	if err != nil {
		return err
	}
	defer db.Close()

	if _, err := db.RemoveCatalog(name); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Removed catalog %q\n", name)
	return nil
}

func runCatalogUpdate(cmd *cobra.Command, name string, opts *catalogUpdateOptions) error {
	ctx := cmd.Context()

	db, err := catalog.OpenDefaultDB()
	if err != nil {
		return err
	}
	defer db.Close()

	var toUpdate []catalog.Catalog
	if name != "" {
		cat, err := db.GetCatalog(name)
		if err != nil {
			return err
		}
		if cat == nil {
			return fmt.Errorf("catalog %q not found", name)
		}
		toUpdate = []catalog.Catalog{*cat}
	} else {
		catalogs, err := db.SortedCatalogs()
		if err != nil {
			return err
		}
		toUpdate = catalogs
	}

	const numWorkers = 3

	// ── Phase 1: Resolve refs and check freshness ──────────────────────
	type resolvedCatalog struct {
		name        string
		newDigest   digest.Digest
		upToDate    bool
		cachingRepo *image.CachingRepository
		err         error
	}

	resolved := make([]resolvedCatalog, len(toUpdate))

	resolveGroup, resolveCtx := errgroup.WithContext(ctx)
	resolveGroup.SetLimit(numWorkers)
	for idx, cat := range toUpdate {
		resolveGroup.Go(func() error {
			res := &resolved[idx]
			res.name = cat.Name

			tRef, err := transport.ParseRef(cat.Ref)
			if err != nil {
				res.err = fmt.Errorf("catalog %q: %w", cat.Name, err)
				return nil
			}

			cachingRepo, err := newDockerCachingRepo(resolveCtx, tRef.Ref)
			if err != nil {
				res.err = fmt.Errorf("catalog %q: %w", cat.Name, err)
				return nil
			}

			desc, err := cachingRepo.Resolve(resolveCtx)
			if err != nil {
				cachingRepo.Close()
				res.err = fmt.Errorf("catalog %q: resolving image: %w", cat.Name, err)
				return nil
			}

			res.newDigest = desc.Digest

			if !opts.force && desc.Digest.String() == cat.Digest {
				res.upToDate = true
				cachingRepo.Close()
				return nil
			}

			// Keep cachingRepo open for phase 2.
			res.cachingRepo = cachingRepo
			return nil
		})
	}
	if err := resolveGroup.Wait(); err != nil {
		for i := range resolved {
			if resolved[i].cachingRepo != nil {
				resolved[i].cachingRepo.Close()
			}
		}
		return err
	}

	// ── Phase 2: Download out-of-date catalogs ─────────────────────────
	p := mpb.New(mpb.WithOutput(cmd.ErrOrStderr()))

	// Create all progress bars upfront so they render together.
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
			defer res.cachingRepo.Close()

			resolver := image.NewResolver()
			resolver.Register(&image.FBCHandler{})

			total, err := resolver.TotalSize(unpackCtx, res.cachingRepo)
			if err != nil {
				res.err = fmt.Errorf("catalog %q: computing total size: %w", res.name, err)
				bar.Abort(true)
				return nil
			}
			bar.SetTotal(total, false)

			res.cachingRepo.SetOnBytesRead(func(n int) { bar.IncrBy(n) })

			tmpDir, err := os.MkdirTemp("", "orb-catalog-*")
			if err != nil {
				res.err = fmt.Errorf("catalog %q: creating temp directory: %w", res.name, err)
				bar.Abort(true)
				return nil
			}
			defer os.RemoveAll(tmpDir)

			if err := resolver.Unpack(unpackCtx, res.cachingRepo, tmpDir); err != nil {
				res.err = fmt.Errorf("catalog %q: unpacking catalog: %w", res.name, err)
				bar.Abort(true)
				return nil
			}

			pkgData, err := catalog.BuildPackageData(unpackCtx, os.DirFS(tmpDir))
			if err != nil {
				res.err = fmt.Errorf("catalog %q: building package data: %w", res.name, err)
				bar.Abort(true)
				return nil
			}

			newDigest := res.newDigest.String()
			if err := db.UpdateCatalog(res.name, func(cat *catalog.Catalog) {
				cat.Digest = newDigest
			}); err != nil {
				res.err = fmt.Errorf("catalog %q: updating catalog: %w", res.name, err)
				bar.Abort(true)
				return nil
			}

			if err := db.SetPackageData(res.name, pkgData); err != nil {
				res.err = fmt.Errorf("catalog %q: saving package data: %w", res.name, err)
				bar.Abort(true)
				return nil
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

	// ── Phase 3: Report results ────────────────────────────────────────
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
			fmt.Fprintf(out, "Catalog %q is up-to-date (%s)\n", res.name, res.newDigest)
		} else {
			fmt.Fprintf(out, "Catalog %q updated (%s)\n", res.name, res.newDigest)
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
}

func newCatalogResolveCmd() *cobra.Command {
	opts := &catalogResolveOptions{}

	cmd := &cobra.Command{
		Use:   "resolve PACKAGE",
		Short: "Resolve matching bundles for a package",
		Long: `Resolve matching bundle versions for a package from configured catalogs.

Catalogs are searched in priority order (highest first). The first catalog
that contains the package is used for resolution. All matching bundles are
returned, sorted by version descending.

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
  orb catalog resolve vault -o jsonpath='{.items[0].image}'`,
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

	return cmd
}

func runCatalogResolve(cmd *cobra.Command, packageName string, opts *catalogResolveOptions) error {
	db, err := catalog.OpenDefaultDB()
	if err != nil {
		return err
	}
	defer db.Close()

	resolveOpts := catalog.ResolveOptions{
		CatalogLabelSelector: opts.catalogLabelSelector,
		Channels:             opts.channels,
		Version:              opts.version,
	}

	if opts.installed != "" {
		name, version, ok := strings.Cut(opts.installed, "=")
		if !ok {
			return fmt.Errorf("invalid --installed value %q: expected format name=version", opts.installed)
		}
		resolveOpts.InstalledName = name
		resolveOpts.InstalledVersion = version
	}

	results, err := catalog.Resolve(db, packageName, resolveOpts)
	if err != nil {
		return err
	}

	return printResolveResults(cmd.OutOrStdout(), results, opts.output)
}

type resolveOutput struct {
	Items []catalog.ResolveResult `json:"items"`
}

func printResolveResults(out io.Writer, results []catalog.ResolveResult, format string) error {
	output := resolveOutput{Items: results}
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
		// Convert through JSON so the jsonpath library sees
		// map[string]interface{} values it can traverse.
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
		w := newTabWriter(out)
		fmt.Fprintln(w, "CATALOG\tVERSION\tIMAGE")
		for _, r := range results {
			fmt.Fprintf(w, "%s\t%s\t%s\n",
				r.CatalogName,
				r.VersionRelease,
				r.Image,
			)
		}
		return w.Flush()
	default:
		return fmt.Errorf("unsupported output format %q: use json, yaml, or jsonpath=TEMPLATE", format)
	}
}

func newDockerCachingRepo(ctx context.Context, ref string) (*image.CachingRepository, error) {
	imgRef, err := dockerTransport.ParseReference("//" + ref)
	if err != nil {
		return nil, fmt.Errorf("parsing docker reference: %w", err)
	}

	sysCtx := &types.SystemContext{}

	client, err := image.NewContainersImageClient(ctx, imgRef, sysCtx)
	if err != nil {
		return nil, fmt.Errorf("creating image client: %w", err)
	}

	repo, err := image.NewCachingRepository(client)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("creating caching repository: %w", err)
	}
	return repo, nil
}
