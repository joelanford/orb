package cmd

import (
	"cmp"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"text/tabwriter"

	"github.com/spf13/cobra"
	dockerTransport "go.podman.io/image/v5/docker"
	"go.podman.io/image/v5/types"

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
	cmd.AddCommand(newCatalogUpdateCmd())

	return cmd
}

type catalogAddOptions struct {
	priority int
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
  orb catalog add operatorhubio docker://quay.io/operatorhubio/catalog:latest --priority 100`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCatalogAdd(cmd, args[0], args[1], opts)
		},
	}

	cmd.Flags().IntVar(&opts.priority, "priority", 0, "Priority for catalog ordering (higher is preferred)")

	return cmd
}

type catalogEditOptions struct {
	priority int
}

func newCatalogEditCmd() *cobra.Command {
	opts := &catalogEditOptions{}

	cmd := &cobra.Command{
		Use:   "edit NAME",
		Short: "Edit catalog settings",
		Long: `Edit settings for an existing catalog.

Examples:
  orb catalog edit operatorhubio --priority 100`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCatalogEdit(cmd, args[0], opts)
		},
	}

	cmd.Flags().IntVar(&opts.priority, "priority", 0, "Priority for catalog ordering (higher is preferred)")

	return cmd
}

func runCatalogEdit(cmd *cobra.Command, name string, opts *catalogEditOptions) error {
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
	configPath, err := catalog.DefaultConfigPath()
	if err != nil {
		return err
	}

	cfg, err := catalog.Load(configPath)
	if err != nil {
		return err
	}

	if len(cfg.Catalogs) == 0 {
		fmt.Fprintln(cmd.OutOrStdout(), "No catalogs configured.")
		return nil
	}

	slices.SortFunc(cfg.Catalogs, func(a, b catalog.Catalog) int {
		if c := cmp.Compare(b.Priority, a.Priority); c != 0 {
			return c
		}
		return cmp.Compare(a.Name, b.Name)
	})

	w := newTabWriter(cmd.OutOrStdout())
	fmt.Fprintln(w, "NAME\tREF\tPRIORITY")
	for _, cat := range cfg.Catalogs {
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
	tRef, err := transport.ParseTransportRef(ref)
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

	ctx := cmd.Context()

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

	desc, err := repo.Resolve(ctx)
	if err != nil {
		return fmt.Errorf("resolving image: %w", err)
	}

	contentDir := filepath.Join(cacheDir, name, desc.Digest.Algorithm().String(), desc.Digest.Encoded())
	if err := os.MkdirAll(contentDir, 0755); err != nil {
		return fmt.Errorf("creating content directory: %w", err)
	}

	resolver := image.NewResolver()
	resolver.Register(&image.FBCHandler{})

	if err := resolver.Unpack(ctx, repo, contentDir); err != nil {
		os.RemoveAll(contentDir)
		return fmt.Errorf("unpacking catalog: %w", err)
	}

	if err := cfg.Add(catalog.Catalog{
		Name:       name,
		Ref:        ref,
		ContentDir: contentDir,
		Priority:   opts.priority,
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

	ctx := cmd.Context()

	for _, catName := range toUpdate {
		cat, _ := cfg.Get(catName)

		tRef, err := transport.ParseTransportRef(cat.Ref)
		if err != nil {
			return fmt.Errorf("catalog %q: %w", catName, err)
		}

		imgRef, err := dockerTransport.ParseReference("//" + tRef.Ref)
		if err != nil {
			return fmt.Errorf("catalog %q: parsing docker reference: %w", catName, err)
		}

		sysCtx := &types.SystemContext{}

		client, err := image.NewContainersImageClient(ctx, imgRef, sysCtx)
		if err != nil {
			return fmt.Errorf("catalog %q: creating image client: %w", catName, err)
		}

		repo, err := image.NewCachingRepository(client)
		if err != nil {
			client.Close()
			return fmt.Errorf("catalog %q: creating caching repository: %w", catName, err)
		}

		desc, err := repo.Resolve(ctx)
		if err != nil {
			repo.Close()
			return fmt.Errorf("catalog %q: resolving image: %w", catName, err)
		}

		newContentDir := filepath.Join(cacheDir, catName, desc.Digest.Algorithm().String(), desc.Digest.Encoded())
		if newContentDir == cat.ContentDir {
			repo.Close()
			fmt.Fprintf(cmd.OutOrStdout(), "Catalog %q is up to date (%s)\n", catName, desc.Digest)
			continue
		}

		if err := os.MkdirAll(newContentDir, 0755); err != nil {
			repo.Close()
			return fmt.Errorf("catalog %q: creating content directory: %w", catName, err)
		}

		resolver := image.NewResolver()
		resolver.Register(&image.FBCHandler{})

		if err := resolver.Unpack(ctx, repo, newContentDir); err != nil {
			repo.Close()
			os.RemoveAll(newContentDir)
			return fmt.Errorf("catalog %q: unpacking catalog: %w", catName, err)
		}
		repo.Close()

		oldContentDir := cat.ContentDir
		cat.ContentDir = newContentDir

		if err := cfg.Save(configPath); err != nil {
			return fmt.Errorf("catalog %q: saving config: %w", catName, err)
		}

		if oldContentDir != "" {
			os.RemoveAll(oldContentDir)
		}

		fmt.Fprintf(cmd.OutOrStdout(), "Updated catalog %q (%s)\n", catName, desc.Digest)
	}

	return nil
}
