package cmd

import (
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	mmsemver "github.com/Masterminds/semver/v3"
	bsemver "github.com/blang/semver/v4"
	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	catalogv1 "github.com/joelanford/library-olm/catalog/v1"
	resolverv1 "github.com/joelanford/library-olm/resolver/v1"
	"github.com/spf13/cobra"
	"helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	chartutil "helm.sh/helm/v4/pkg/chart/v2/util"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/release"
	"k8s.io/apimachinery/pkg/labels"

	helmplugins "github.com/joelanford/orb/helm-plugins"
	"github.com/joelanford/orb/internal/helm"
	"github.com/joelanford/orb/internal/source"
	"github.com/joelanford/orb/internal/transport"
)

var pluginRunners = map[string]func(*cobra.Command, []string) error{
	"orb-getter": runHelmPluginOrbGetterCmd,
}

func newHelmPluginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "helm-plugin",
		Short: "Helm getter plugin commands",
	}
	cmd.AddCommand(newHelmPluginInstallCmd())
	cmd.AddCommand(newHelmPluginUninstallCmd())
	cmd.AddCommand(newHelmPluginRunCmd())
	return cmd
}

func newHelmPluginInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install a Helm plugin managed by orb",
	}
	for _, name := range pluginNames() {
		name := name
		cmd.AddCommand(&cobra.Command{
			Use:   name,
			Short: fmt.Sprintf("Install the %s Helm plugin", name),
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				pluginsDir := cli.New().PluginsDirectory
				if err := installPlugin(pluginsDir, name); err != nil {
					return fmt.Errorf("installing plugin %q: %w", name, err)
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "Installed Helm plugin %q to %s\n", name, filepath.Join(pluginsDir, name))
				return nil
			},
		})
	}
	return cmd
}

func newHelmPluginUninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Uninstall a Helm plugin managed by orb",
	}
	for _, name := range pluginNames() {
		name := name
		cmd.AddCommand(&cobra.Command{
			Use:   name,
			Short: fmt.Sprintf("Uninstall the %s Helm plugin", name),
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				pluginsDir := cli.New().PluginsDirectory
				dest := filepath.Join(pluginsDir, name)
				if err := os.RemoveAll(dest); err != nil {
					return fmt.Errorf("uninstalling plugin %q: %w", name, err)
				}
				fmt.Fprintf(cmd.ErrOrStderr(), "Uninstalled Helm plugin %q from %s\n", name, dest)
				return nil
			},
		})
	}
	return cmd
}

func newHelmPluginRunCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run <pluginName> [args...]",
		Short: "Run an embedded Helm plugin",
		Long: `Run an embedded Helm plugin by name.

This command is typically invoked by a plugin's get.sh script rather than
called directly by users.

Examples:
  orb helm-plugin run orb-getter "orb://vault/"
  orb helm-plugin run orb-getter "orb://vault/?version=^1.0"`,
		Args:               cobra.MinimumNArgs(1),
		DisableFlagParsing: true,
		SilenceUsage:       true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Since flag parsing is disabled, handle --help manually.
			for _, a := range args {
				if a == "--help" || a == "-h" {
					return cmd.Help()
				}
			}
			if len(args) == 0 {
				return cmd.Help()
			}
			pluginName := args[0]
			runner, ok := pluginRunners[pluginName]
			if !ok {
				return fmt.Errorf("unknown plugin %q", pluginName)
			}
			return runner(cmd, args[1:])
		},
	}
	return cmd
}

// pluginNames returns the names of all embedded plugin directories.
func pluginNames() []string {
	entries, err := fs.ReadDir(helmplugins.FS, ".")
	if err != nil {
		// Embedded FS read should never fail.
		panic(fmt.Sprintf("reading embedded plugins: %v", err))
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names
}

// installPlugin copies the embedded plugin files into the Helm plugins directory.
func installPlugin(pluginsDir, name string) error {
	dest := filepath.Join(pluginsDir, name)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}

	return fs.WalkDir(helmplugins.FS, name, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		target := filepath.Join(pluginsDir, path)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}

		data, err := fs.ReadFile(helmplugins.FS, path)
		if err != nil {
			return err
		}

		perm := os.FileMode(0o644)
		if strings.HasSuffix(path, ".sh") {
			perm = 0o755
		}

		return os.WriteFile(target, data, perm)
	})
}

func runHelmPluginOrbGetterCmd(cmd *cobra.Command, args []string) error {
	if len(args) < 1 || len(args) > 4 {
		return fmt.Errorf("expected 1 to 4 arguments, got %d", len(args))
	}
	// Helm subprocess getter passes: certFile keyFile caFile URL
	// Standalone passes: URL
	rawURL := args[len(args)-1]
	return runHelmPluginOrbGetter(cmd, rawURL)
}

// orbURL holds the parsed components of an orb:// URL.
type orbURL struct {
	PackageName          string
	Version              string
	Channels             []string
	CatalogLabelSelector string
	ReleaseName          string
}

// parseOrbURL parses an orb://<packageName>[?<query-parameters>] URL.
func parseOrbURL(rawURL string) (*orbURL, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("parsing orb URL: %w", err)
	}
	if u.Scheme != "orb" {
		return nil, fmt.Errorf("expected orb:// scheme, got %q", u.Scheme)
	}

	packageName := u.Host
	if packageName == "" {
		packageName = u.Opaque
	}
	if packageName == "" {
		return nil, fmt.Errorf("package name is required in orb:// URL")
	}

	q := u.Query()
	return &orbURL{
		PackageName:          packageName,
		Version:              q.Get("version"),
		Channels:             q["channel"],
		CatalogLabelSelector: q.Get("catalog-label-selector"),
		ReleaseName:          q.Get("release"),
	}, nil
}

// detectInstalled uses the Helm SDK to find a currently installed release
// matching the given package name. It returns the installed bundle name
// (from the chart's BundleNameAnnotation) and version for passing to the
// catalog resolver as InstalledName/InstalledVersion.
func detectInstalled(packageName, releaseName string) (name string, version string, err error) {
	settings := cli.New()

	cfg := action.NewConfiguration()
	if err := cfg.Init(settings.RESTClientGetter(), settings.Namespace(), ""); err != nil {
		// If we can't connect to a cluster, treat as fresh install.
		return "", "", nil
	}

	list := action.NewList(cfg)
	list.All = true
	list.StateMask = action.ListDeployed | action.ListFailed | action.ListPendingInstall | action.ListPendingUpgrade | action.ListPendingRollback
	releases, err := list.Run()
	if err != nil {
		// If listing fails (e.g., no cluster), treat as fresh install.
		return "", "", nil
	}

	// Filter to releases whose chart name matches the package name.
	type matchedRelease struct {
		releaseName string
		bundleName  string
		version     string
	}
	var matches []matchedRelease
	for _, rel := range releases {
		accessor, err := release.NewAccessor(rel)
		if err != nil {
			continue
		}
		charter := accessor.Chart()
		if charter == nil {
			continue
		}
		ch, ok := charter.(*chart.Chart)
		if !ok || ch.Metadata == nil {
			continue
		}
		if ch.Metadata.Name != packageName {
			continue
		}

		bundleName := ch.Metadata.Annotations[helm.BundleNameAnnotation]

		matches = append(matches, matchedRelease{
			releaseName: accessor.Name(),
			bundleName:  bundleName,
			version:     ch.Metadata.Version,
		})
	}

	switch {
	case len(matches) == 0:
		// Fresh install — no installed version constraints.
		return "", "", nil
	case len(matches) == 1:
		return matches[0].bundleName, matches[0].version, nil
	default:
		// Multiple releases with same chart name — need disambiguation.
		if releaseName == "" {
			return "", "", fmt.Errorf(
				"multiple releases found for package %q; "+
					"use the release query parameter to disambiguate (e.g. orb://%s/?release=<name>)",
				packageName, packageName,
			)
		}
		for _, m := range matches {
			if m.releaseName == releaseName {
				return m.bundleName, m.version, nil
			}
		}
		return "", "", fmt.Errorf("release %q not found for package %q", releaseName, packageName)
	}
}

func runHelmPluginOrbGetter(cmd *cobra.Command, rawURL string) error {
	ctx := cmd.Context()

	// 1. Parse the orb:// URL.
	oURL, err := parseOrbURL(rawURL)
	if err != nil {
		return err
	}

	// 2. Detect installed release via Helm SDK.
	installedName, installedVersion, err := detectInstalled(oURL.PackageName, oURL.ReleaseName)
	if err != nil {
		return fmt.Errorf("detecting installed release: %w", err)
	}

	// 3. Resolve from catalog.
	store, err := openStore()
	if err != nil {
		return fmt.Errorf("opening catalog database: %w", err)
	}
	defer store.Close()

	var resolveOpts []resolverv1.ResolveOption
	var reader catalogv1.StoreReader = store

	if oURL.CatalogLabelSelector != "" {
		selector, err := labels.Parse(oURL.CatalogLabelSelector)
		if err != nil {
			return fmt.Errorf("parsing catalog label selector: %w", err)
		}
		reader = store.Select(selector)
	}
	if len(oURL.Channels) > 0 {
		paths := make([][]string, len(oURL.Channels))
		for i, ch := range oURL.Channels {
			paths[i] = []string{ch}
		}
		resolveOpts = append(resolveOpts, resolverv1.WithGraphs(paths))
	}
	if oURL.Version != "" {
		constraint, err := mmsemver.NewConstraint(oURL.Version)
		if err != nil {
			return fmt.Errorf("parsing version constraint: %w", err)
		}
		resolveOpts = append(resolveOpts, resolverv1.WithMastermindsVersionConstraint(*constraint))
	}
	if installedName != "" && installedVersion != "" {
		v, err := bsemver.Parse(installedVersion)
		if err != nil {
			return fmt.Errorf("parsing installed version %q: %w", installedVersion, err)
		}
		resolveOpts = append(resolveOpts, resolverv1.WithSuccessorsOf(installedBundleIdentity{
			id:  bundlev1.BundleID(installedName),
			nvr: bundlev1.NameVersionRelease{Version: v},
		}))
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "orb: resolving package %q (version=%q, channels=%v, catalogLabelSelector=%q, installedName=%q, installedVersion=%q)\n",
		oURL.PackageName, oURL.Version, oURL.Channels, oURL.CatalogLabelSelector, installedName, installedVersion)

	cat, bundles, err := resolverv1.Resolve(ctx, reader, oURL.PackageName, resolveOpts...)
	if err != nil {
		return fmt.Errorf("resolving package: %w", err)
	}
	if cat == nil || len(bundles) == 0 {
		return fmt.Errorf("no matching bundle found for package %q", oURL.PackageName)
	}

	best := bundles[0]
	fmt.Fprintf(cmd.ErrOrStderr(), "orb: resolved bundle %q (version %q)\n", best.ID(), best.NameVersionRelease().Version)

	// 4. Read the bundle from the resolved image.
	srcRef, err := transport.ParseRef("docker://" + best.URI())
	if err != nil {
		return fmt.Errorf("parsing image reference: %w", err)
	}

	src, err := source.New(srcRef, source.Options{})
	if err != nil {
		return fmt.Errorf("creating source: %w", err)
	}

	b, err := src.Read(ctx)
	if err != nil {
		return fmt.Errorf("reading bundle: %w", err)
	}

	// 5. Convert to a Helm chart archive.
	c, err := helm.Generate(b)
	if err != nil {
		return fmt.Errorf("generating helm chart: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "orb-helm-plugin-*")
	if err != nil {
		return fmt.Errorf("creating temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	archivePath, err := chartutil.Save(c, tmpDir)
	if err != nil {
		return fmt.Errorf("saving chart archive: %w", err)
	}

	// 6. Write .tgz contents to stdout.
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("opening chart archive: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(cmd.OutOrStdout(), f); err != nil {
		return fmt.Errorf("writing chart to stdout: %w", err)
	}

	return nil
}
