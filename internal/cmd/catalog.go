package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	cmd.AddCommand(newCatalogListCmd())
	cmd.AddCommand(newCatalogRemoveCmd())
	cmd.AddCommand(newCatalogResolveCmd())
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
	ctx := cmd.Context()
	_ = ctx

	configPath, err := catalog.DefaultConfigPath()
	if err != nil {
		return err
	}

	cfg, err := catalog.Load(configPath)
	if err != nil {
		return err
	}

	cat, ok := cfg.Get(name)
	if !ok {
		return fmt.Errorf("catalog %q not found", name)
	}

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

	if err := cfg.Save(configPath); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Edited catalog %q\n", name)
	return nil
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
	ctx := cmd.Context()
	_ = ctx

	configPath, err := catalog.DefaultConfigPath()
	if err != nil {
		return err
	}

	cfg, err := catalog.Load(configPath)
	if err != nil {
		return err
	}

	catalogs := cfg.SortedCatalogs()
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

func newCatalogUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update [NAME]",
		Short: "Update one or all catalogs",
		Long: `Update catalogs by re-pulling FBC content from their OCI images.

If NAME is given, only that catalog is updated. Otherwise all catalogs are updated.
If the resolved digest has not changed, the pull is skipped.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var name string
			if len(args) > 0 {
				name = args[0]
			}
			return runCatalogUpdate(cmd, name)
		},
	}
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

	configPath, err := catalog.DefaultConfigPath()
	if err != nil {
		return err
	}

	cfg, err := catalog.Load(configPath)
	if err != nil {
		return err
	}

	if _, ok := cfg.Get(name); ok {
		return fmt.Errorf("catalog %q already exists", name)
	}

	cacheDir, err := catalog.DefaultCacheDir()
	if err != nil {
		return err
	}

	imgRef, err := dockerTransport.ParseReference("//" + tRef.Ref)
	if err != nil {
		return fmt.Errorf("parsing docker reference: %w", err)
	}

	sysCtx := &types.SystemContext{}

	client, err := image.NewContainersImageClient(ctx, imgRef, sysCtx)
	if err != nil {
		return fmt.Errorf("creating image client: %w", err)
	}

	repo, err := image.NewCachingRepository(client)
	if err != nil {
		client.Close()
		return fmt.Errorf("creating caching repository: %w", err)
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

	contentDir := filepath.Join(cacheDir, name, desc.Digest.Algorithm().String(), desc.Digest.Encoded())
	if err := os.MkdirAll(contentDir, 0755); err != nil {
		bar.Abort(true)
		p.Wait()
		return fmt.Errorf("creating content directory: %w", err)
	}

	if err := resolver.Unpack(ctx, repo, contentDir); err != nil {
		os.RemoveAll(contentDir)
		bar.Abort(true)
		p.Wait()
		return fmt.Errorf("unpacking catalog: %w", err)
	}

	bar.SetTotal(total, true)
	p.Wait()

	if err := cfg.Add(catalog.Catalog{
		Name:       name,
		Ref:        ref,
		ContentDir: contentDir,
		Priority:   opts.priority,
		Labels:     opts.labels,
	}); err != nil {
		os.RemoveAll(contentDir)
		return err
	}

	if err := cfg.Save(configPath); err != nil {
		os.RemoveAll(contentDir)
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Added catalog %q (%s)\n", name, desc.Digest)
	return nil
}

func runCatalogRemove(cmd *cobra.Command, name string) error {
	ctx := cmd.Context()
	_ = ctx

	configPath, err := catalog.DefaultConfigPath()
	if err != nil {
		return err
	}

	cfg, err := catalog.Load(configPath)
	if err != nil {
		return err
	}

	removed, err := cfg.Remove(name)
	if err != nil {
		return err
	}

	if removed.ContentDir != "" {
		// Remove the name-level directory to clean up all digest subdirs
		cacheDir, err := catalog.DefaultCacheDir()
		if err == nil {
			os.RemoveAll(filepath.Join(cacheDir, name))
		}
	}

	if err := cfg.Save(configPath); err != nil {
		return err
	}

	fmt.Fprintf(cmd.OutOrStdout(), "Removed catalog %q\n", name)
	return nil
}

func runCatalogUpdate(cmd *cobra.Command, name string) error {
	ctx := cmd.Context()

	configPath, err := catalog.DefaultConfigPath()
	if err != nil {
		return err
	}

	cfg, err := catalog.Load(configPath)
	if err != nil {
		return err
	}

	cacheDir, err := catalog.DefaultCacheDir()
	if err != nil {
		return err
	}

	var toUpdate []string
	if name != "" {
		if _, ok := cfg.Get(name); !ok {
			return fmt.Errorf("catalog %q not found", name)
		}
		toUpdate = []string{name}
	} else {
		for _, cat := range cfg.Catalogs {
			toUpdate = append(toUpdate, cat.Name)
		}
	}

	const numWorkers = 3

	// ── Phase 1: Resolve refs and check freshness ──────────────────────
	type resolvedCatalog struct {
		name              string
		digest            digest.Digest
		newContentDir     string
		currentContentDir string
		upToDate          bool
		cachingRepo       *image.CachingRepository
		err               error
	}

	resolved := make([]resolvedCatalog, len(toUpdate))

	resolveGroup, resolveCtx := errgroup.WithContext(ctx)
	resolveGroup.SetLimit(numWorkers)
	for idx, catName := range toUpdate {
		cat, _ := cfg.Get(catName)
		resolveGroup.Go(func() error {
			res := &resolved[idx]
			res.name = catName
			res.currentContentDir = cat.ContentDir

			tRef, err := transport.ParseRef(cat.Ref)
			if err != nil {
				res.err = fmt.Errorf("catalog %q: %w", catName, err)
				return nil
			}

			imgRef, err := dockerTransport.ParseReference("//" + tRef.Ref)
			if err != nil {
				res.err = fmt.Errorf("catalog %q: parsing docker reference: %w", catName, err)
				return nil
			}

			sysCtx := &types.SystemContext{}

			client, err := image.NewContainersImageClient(resolveCtx, imgRef, sysCtx)
			if err != nil {
				res.err = fmt.Errorf("catalog %q: creating image client: %w", catName, err)
				return nil
			}

			cachingRepo, err := image.NewCachingRepository(client)
			if err != nil {
				client.Close()
				res.err = fmt.Errorf("catalog %q: creating caching repository: %w", catName, err)
				return nil
			}

			desc, err := cachingRepo.Resolve(resolveCtx)
			if err != nil {
				cachingRepo.Close()
				res.err = fmt.Errorf("catalog %q: resolving image: %w", catName, err)
				return nil
			}

			res.digest = desc.Digest
			res.newContentDir = filepath.Join(cacheDir, catName, desc.Digest.Algorithm().String(), desc.Digest.Encoded())

			if res.newContentDir == cat.ContentDir {
				if _, err := os.Stat(res.newContentDir); err == nil {
					res.upToDate = true
					cachingRepo.Close()
					return nil
				}
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

			if err := os.MkdirAll(res.newContentDir, 0755); err != nil {
				res.err = fmt.Errorf("catalog %q: creating content directory: %w", res.name, err)
				bar.Abort(true)
				return nil
			}

			if err := resolver.Unpack(unpackCtx, res.cachingRepo, res.newContentDir); err != nil {
				os.RemoveAll(res.newContentDir)
				res.err = fmt.Errorf("catalog %q: unpacking catalog: %w", res.name, err)
				bar.Abort(true)
				return nil
			}

			// Save config immediately so a subsequent run won't re-pull
			// this catalog if a later catalog fails.
			if err := cfg.UpdateAndSave(res.name, configPath, func(cat *catalog.Catalog) {
				cat.ContentDir = res.newContentDir
			}); err != nil {
				os.RemoveAll(res.newContentDir)
				res.err = fmt.Errorf("catalog %q: saving config: %w", res.name, err)
				bar.Abort(true)
				return nil
			}
			if res.currentContentDir != "" && res.currentContentDir != res.newContentDir {
				os.RemoveAll(res.currentContentDir)
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
			fmt.Fprintf(out, "Catalog %q is up-to-date (%s)\n", res.name, res.digest)
		} else {
			fmt.Fprintf(out, "Catalog %q updated (%s)\n", res.name, res.digest)
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
	ctx := cmd.Context()
	_ = ctx

	configPath, err := catalog.DefaultConfigPath()
	if err != nil {
		return err
	}

	cfg, err := catalog.Load(configPath)
	if err != nil {
		return err
	}

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

	results, err := catalog.Resolve(cfg, packageName, resolveOpts)
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
		fmt.Fprintln(w, "CATALOG\tPACKAGE\tCHANNELS\tBUNDLE\tVERSION\tIMAGE")
		for _, r := range results {
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				r.CatalogName,
				r.PackageName,
				r.ChannelsString(),
				r.BundleName,
				r.Version,
				r.Image,
			)
		}
		return w.Flush()
	default:
		return fmt.Errorf("unsupported output format %q: use json, yaml, or jsonpath=TEMPLATE", format)
	}
}
