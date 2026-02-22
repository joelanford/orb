package catalog

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"
	bsemver "github.com/blang/semver/v4"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/property"
	"k8s.io/apimachinery/pkg/labels"
)

// ResolveOptions holds constraints for bundle resolution.
type ResolveOptions struct {
	CatalogLabelSelector string   // Kubernetes label selector string (parsed via labels.Parse)
	Channels             []string // Channel names to search (empty = all channels)
	Version              string   // Masterminds semver constraint
	InstalledName        string   // Installed bundle name (for successor filtering)
	InstalledVersion     string   // Installed bundle version (for skipRange matching)
}

// ResolveResult holds a resolved bundle and all channels it appears in.
type ResolveResult struct {
	CatalogName string   `json:"catalog"`
	PackageName string   `json:"package"`
	Channels    []string `json:"channels"`
	BundleName  string   `json:"bundle"`
	Version     string   `json:"version"`
	Image       string   `json:"image"`
}

// Resolve iterates catalogs in priority order, returning all matching bundles
// from the first catalog that contains the package, sorted by version descending.
func Resolve(cfg *Config, packageName string, opts ResolveOptions) ([]ResolveResult, error) {
	var selector labels.Selector
	if opts.CatalogLabelSelector != "" {
		var err error
		selector, err = labels.Parse(opts.CatalogLabelSelector)
		if err != nil {
			return nil, fmt.Errorf("parsing catalog label selector: %w", err)
		}
	}

	var versionConstraint *semver.Constraints
	if opts.Version != "" {
		var err error
		versionConstraint, err = semver.NewConstraint(opts.Version)
		if err != nil {
			return nil, fmt.Errorf("parsing version constraint: %w", err)
		}
	}

	var installedVersion *bsemver.Version
	if opts.InstalledVersion != "" {
		v, err := bsemver.Parse(opts.InstalledVersion)
		if err != nil {
			return nil, fmt.Errorf("parsing installed version %q: %w", opts.InstalledVersion, err)
		}
		installedVersion = &v
	}

	ctx := context.Background()

	catalogs := cfg.SortedCatalogs()
	for _, cat := range catalogs {
		if selector != nil {
			effectiveLabels := make(map[string]string, len(cat.Labels)+1)
			for k, v := range cat.Labels {
				effectiveLabels[k] = v
			}
			effectiveLabels["olm.operatorframework.io/metadata.name"] = cat.Name
			if !selector.Matches(labels.Set(effectiveLabels)) {
				continue
			}
		}

		packageDir := filepath.Join(cat.ContentDir, "configs", packageName)
		if _, err := os.Stat(packageDir); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("checking package dir for %q in catalog %q: %w", packageName, cat.Name, err)
		}

		fbc, err := declcfg.LoadFS(ctx, os.DirFS(packageDir))
		if err != nil {
			return nil, fmt.Errorf("loading FBC for package %q in catalog %q: %w", packageName, cat.Name, err)
		}

		results, err := resolveFromCatalog(cat, packageName, fbc, opts.Channels, versionConstraint, opts.InstalledName, installedVersion)
		if err != nil {
			return nil, fmt.Errorf("catalog %q: %w", cat.Name, err)
		}
		return results, nil
	}

	return nil, fmt.Errorf("package %q not found in any catalog", packageName)
}

func resolveFromCatalog(
	cat Catalog,
	packageName string,
	fbc *declcfg.DeclarativeConfig,
	channels []string,
	versionConstraint *semver.Constraints,
	installedName string,
	installedVersion *bsemver.Version,
) ([]ResolveResult, error) {
	// Build bundle lookup map
	bundleMap := make(map[string]declcfg.Bundle)
	for _, b := range fbc.Bundles {
		bundleMap[b.Name] = b
	}

	// Determine which channels to search
	var channelsToSearch []declcfg.Channel
	if len(channels) > 0 {
		requested := make(map[string]bool, len(channels))
		for _, ch := range channels {
			requested[ch] = true
		}
		for _, ch := range fbc.Channels {
			if requested[ch.Name] {
				channelsToSearch = append(channelsToSearch, ch)
			}
		}
		if len(channelsToSearch) == 0 {
			return nil, fmt.Errorf("no matching channels found for package %q", packageName)
		}
	} else {
		channelsToSearch = fbc.Channels
	}

	// Collect all matching bundles, tracking which channels each appears in.
	// Key: bundle name, Value: result with parsed version for sorting.
	type candidate struct {
		result  ResolveResult
		semver  *semver.Version
		chanSet map[string]struct{}
	}
	candidates := make(map[string]*candidate)

	for _, ch := range channelsToSearch {
		for _, entry := range ch.Entries {
			b, ok := bundleMap[entry.Name]
			if !ok {
				continue
			}

			ver, err := bundleVersion(b)
			if err != nil {
				continue
			}

			sv, err := semver.NewVersion(ver)
			if err != nil {
				continue
			}

			if versionConstraint != nil && !versionConstraint.Check(sv) {
				continue
			}

			if installedName != "" && installedVersion != nil {
				if !isSuccessor(entry, installedName, *installedVersion) {
					continue
				}
			}

			if c, ok := candidates[entry.Name]; ok {
				c.chanSet[ch.Name] = struct{}{}
			} else {
				candidates[entry.Name] = &candidate{
					result: ResolveResult{
						CatalogName: cat.Name,
						PackageName: packageName,
						BundleName:  entry.Name,
						Version:     ver,
						Image:       b.Image,
					},
					semver:  sv,
					chanSet: map[string]struct{}{ch.Name: {}},
				}
			}
		}
	}

	if len(candidates) == 0 {
		return nil, fmt.Errorf("no bundle found matching constraints for package %q", packageName)
	}

	// Build sorted results: version descending, then bundle name ascending for tie-breaking.
	results := make([]ResolveResult, 0, len(candidates))
	sorted := make([]*candidate, 0, len(candidates))
	for _, c := range candidates {
		sorted = append(sorted, c)
	}
	slices.SortFunc(sorted, func(a, b *candidate) int {
		if vc := b.semver.Compare(a.semver); vc != 0 {
			return vc
		}
		return cmp.Compare(a.result.BundleName, b.result.BundleName)
	})
	for _, c := range sorted {
		chans := make([]string, 0, len(c.chanSet))
		for ch := range c.chanSet {
			chans = append(chans, ch)
		}
		slices.Sort(chans)
		c.result.Channels = chans
		results = append(results, c.result)
	}

	return results, nil
}

func isSuccessor(candidate declcfg.ChannelEntry, installedName string, installedVersion bsemver.Version) bool {
	// Direct replacement
	if candidate.Replaces == installedName {
		return true
	}

	// Explicit skip
	if slices.Contains(candidate.Skips, installedName) {
		return true
	}

	// SkipRange (blang/semver range)
	if candidate.SkipRange != "" {
		rng, err := bsemver.ParseRange(candidate.SkipRange)
		if err == nil && rng(installedVersion) {
			return true
		}
	}

	return false
}

func bundleVersion(b declcfg.Bundle) (string, error) {
	for _, p := range b.Properties {
		if p.Type != property.TypePackage {
			continue
		}
		var pkg property.Package
		if err := json.Unmarshal(p.Value, &pkg); err != nil {
			return "", fmt.Errorf("unmarshaling package property: %w", err)
		}
		return pkg.Version, nil
	}
	return "", fmt.Errorf("bundle %q has no olm.package property", b.Name)
}

// ChannelsString returns a comma-separated string of channel names.
func (r ResolveResult) ChannelsString() string {
	return strings.Join(r.Channels, ",")
}
