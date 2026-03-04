package cmd

import (
	"fmt"
	"io"
	"net/url"
	"os"

	"github.com/spf13/cobra"
	"helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	chartutil "helm.sh/helm/v4/pkg/chart/v2/util"
	"helm.sh/helm/v4/pkg/cli"
	"helm.sh/helm/v4/pkg/release"

	"github.com/joelanford/orb/internal/catalog"
	"github.com/joelanford/orb/internal/helm"
	"github.com/joelanford/orb/internal/source"
	"github.com/joelanford/orb/internal/transport"
)

func newHelmPluginCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "helm-plugin",
		Short: "Helm getter plugin commands",
	}
	cmd.AddCommand(newHelmPluginOrbGetterCmd())
	return cmd
}

func newHelmPluginOrbGetterCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "orb-getter [CERTFILE KEYFILE CAFILE] URL",
		Short: "Fetch a Helm chart from an orb-managed catalog",
		Long: `Fetch a Helm chart from an orb-managed catalog using an orb:// URL.

Parses the orb:// URL, detects any currently installed release for the package
using the Helm SDK, resolves the best version from the catalog, converts the
bundle to a Helm chart archive, and writes the .tgz contents to stdout.

When invoked by the Helm subprocess getter runtime, four positional arguments
are passed: certFile, keyFile, caFile, and the URL. When invoked standalone
for debugging, only the URL is required.

The URL format is orb://packageName/ (trailing slash required for Helm
compatibility). Query parameters control resolution.

Examples:
  # Standalone usage
  orb helm-plugin orb-getter "orb://vault/"
  orb helm-plugin orb-getter "orb://vault/?version=^1.0"
  orb helm-plugin orb-getter "orb://vault/?channel=stable"

  # Helm usage (the plugin is invoked automatically)
  helm install my-release "orb://vault/"
  helm install my-release "orb://vault/?version=^1.0"
  helm upgrade my-release "orb://vault/?version=^1.0"`,
		Args: cobra.RangeArgs(1, 4),
		RunE: func(cmd *cobra.Command, args []string) error {
			// Helm subprocess getter passes: certFile keyFile caFile URL
			// Standalone passes: URL
			rawURL := args[len(args)-1]
			return runHelmPluginOrbGetter(cmd, rawURL)
		},
	}
	return cmd
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
	db, err := catalog.OpenDefaultDB()
	if err != nil {
		return fmt.Errorf("opening catalog database: %w", err)
	}
	defer db.Close()

	resolveOpts := catalog.ResolveOptions{
		Version:              oURL.Version,
		Channels:             oURL.Channels,
		CatalogLabelSelector: oURL.CatalogLabelSelector,
		InstalledName:        installedName,
		InstalledVersion:     installedVersion,
	}
	fmt.Fprintf(cmd.ErrOrStderr(), "orb: resolving package %q (version=%q, channels=%v, catalogLabelSelector=%q, installedName=%q, installedVersion=%q)\n",
		oURL.PackageName, resolveOpts.Version, resolveOpts.Channels, resolveOpts.CatalogLabelSelector, resolveOpts.InstalledName, resolveOpts.InstalledVersion)

	results, err := catalog.Resolve(db, oURL.PackageName, resolveOpts)
	if err != nil {
		return fmt.Errorf("resolving package: %w", err)
	}
	if len(results) == 0 {
		return fmt.Errorf("no matching bundle found for package %q", oURL.PackageName)
	}

	// Use the highest-versioned result (results are sorted descending).
	best := results[0]
	fmt.Fprintf(cmd.ErrOrStderr(), "orb: resolved bundle %q (version %q)\n", best.Name, best.Version)

	// 4. Read the bundle from the resolved image.
	srcRef, err := transport.ParseRef("docker://" + best.Image)
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
