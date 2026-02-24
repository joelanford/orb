package catalog

import (
	"fmt"
	"slices"
	"strings"

	mmsemver "github.com/Masterminds/semver/v3"
	bsemver "github.com/blang/semver/v4"
	"github.com/samber/lo"
	"k8s.io/apimachinery/pkg/labels"
)

// ResolveOptions holds constraints for bundle resolution.
type ResolveOptions struct {
	CatalogLabelSelector string   // Kubernetes label selector string (parsed via labels.Parse)
	Channels             []string // Channel names to search (empty = all channels)
	Version              string   // Masterminds mmsemver constraint
	InstalledName        string   // Installed bundle name (for successor filtering)
	InstalledVersion     string   // Installed bundle version (for skipRange matching)
}

// ResolveResult holds a resolved bundle and all channels it appears in.
type ResolveResult struct {
	CatalogName string   `json:"catalog"`
	PackageName string   `json:"package"`
	Channels    []string `json:"channels"`
	BundleData  `json:",inline"`
}

// Resolve iterates catalogs in priority order, returning all matching bundles
// from the first catalog that contains the package, sorted by version descending.
func Resolve(db *DB, packageName string, opts ResolveOptions) ([]ResolveResult, error) {
	var selector labels.Selector
	if opts.CatalogLabelSelector != "" {
		var err error
		selector, err = labels.Parse(opts.CatalogLabelSelector)
		if err != nil {
			return nil, fmt.Errorf("parsing catalog label selector: %w", err)
		}
	}

	var versionConstraint bsemver.Range
	if opts.Version != "" {
		var err error
		versionConstraint, err = mastermindsConstraintToBlangRange(opts.Version)
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

	catalogs, err := db.SortedCatalogs()
	if err != nil {
		return nil, fmt.Errorf("listing catalogs: %w", err)
	}

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

		pkgData, err := db.GetPackageData(cat.Name, packageName)
		if err != nil {
			return nil, fmt.Errorf("getting package data for %q from catalog %q: %w", packageName, cat.Name, err)
		}
		if pkgData == nil {
			continue
		}

		results, err := resolveFromPackageData(cat, packageName, pkgData, opts.Channels, versionConstraint, opts.InstalledName, installedVersion)
		if err != nil {
			return nil, fmt.Errorf("catalog %q: %w", cat.Name, err)
		}
		return results, nil
	}

	return nil, fmt.Errorf("package %q not found in any catalog", packageName)
}

func resolveFromPackageData(
	cat Catalog,
	packageName string,
	pkgData *PackageData,
	channels []string,
	versionConstraint bsemver.Range,
	installedName string,
	installedVersion *bsemver.Version,
) ([]ResolveResult, error) {
	// Build bundle lookup map
	bundleMap := make(map[string]BundleData)
	for _, b := range pkgData.Bundles {
		bundleMap[b.Name] = b
	}

	// Determine which channels to search
	var channelsToSearch []ChannelData
	if len(channels) > 0 {
		requested := make(map[string]bool, len(channels))
		for _, ch := range channels {
			requested[ch] = true
		}
		for _, ch := range pkgData.Channels {
			if requested[ch.Name] {
				channelsToSearch = append(channelsToSearch, ch)
			}
		}
		if len(channelsToSearch) == 0 {
			return nil, fmt.Errorf("no matching channels found for package %q", packageName)
		}
	} else {
		channelsToSearch = pkgData.Channels
	}

	// Collect all matching bundles, tracking which channels each appears in.
	type candidate struct {
		result  ResolveResult
		chanSet map[string]struct{}
	}
	candidates := make(map[string]*candidate)

	for _, ch := range channelsToSearch {
		for _, entry := range ch.Entries {
			b, ok := bundleMap[entry.Name]
			if !ok {
				continue
			}

			if versionConstraint != nil && !versionConstraint(b.Version) {
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
						BundleData:  b,
					},
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
		return CompareBundleData(b.result.BundleData, a.result.BundleData)
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

func isSuccessor(candidate EntryData, installedName string, installedVersion bsemver.Version) bool {
	// Direct replacement
	if candidate.Replaces == installedName {
		return true
	}

	// Explicit skip
	if slices.Contains(candidate.Skips, installedName) {
		return true
	}

	// SkipRange (blang/mmsemver range)
	if candidate.SkipRange != "" {
		rng, err := bsemver.ParseRange(candidate.SkipRange)
		if err == nil && rng(installedVersion) {
			return true
		}
	}

	return false
}

// ChannelsString returns a comma-separated string of channel names.
func (r ResolveResult) ChannelsString() string {
	return strings.Join(r.Channels, ",")
}

func mastermindsConstraintToBlangRange(constraintStr string) (bsemver.Range, error) {
	c, err := mmsemver.NewConstraint(constraintStr)
	if err != nil {
		return nil, fmt.Errorf("parsing constraint %q: %w", constraintStr, err)
	}
	return func(v bsemver.Version) bool {
		pre := lo.Map(v.Pre, func(p bsemver.PRVersion, _ int) string { return p.String() })
		mmv := mmsemver.New(v.Major, v.Minor, v.Patch, strings.Join(pre, "."), strings.Join(v.Build, "."))
		return c.Check(mmv)
	}, nil
}
