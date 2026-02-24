package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"sync"

	"github.com/joelanford/orb/internal/bundle"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/property"
)

// PackageData holds pre-processed resolve data for a single package.
type PackageData struct {
	DisplayName string        `json:"displayName,omitempty"`
	Description string        `json:"description,omitempty"`
	Keywords    []string      `json:"keywords,omitempty"`
	Icon        *Icon         `json:"icon,omitempty"`
	Channels    []ChannelData `json:"channels"`
	Bundles     []BundleData  `json:"bundles"`
}

// Icon holds a package icon.
type Icon struct {
	Data      []byte `json:"base64data"`
	MediaType string `json:"mediatype"`
}

// ChannelData holds channel metadata for resolution.
type ChannelData struct {
	Name    string      `json:"name"`
	Entries []EntryData `json:"entries"`
}

// EntryData holds a channel entry for resolution.
type EntryData struct {
	Name      string   `json:"name"`
	Replaces  string   `json:"replaces,omitempty"`
	Skips     []string `json:"skips,omitempty"`
	SkipRange string   `json:"skipRange,omitempty"`
}

// BundleData holds pre-extracted bundle metadata needed for resolution.
// Version is extracted from the olm.package property at add/update time
// so the resolver never needs to parse bundle properties.
type BundleData struct {
	Name                  string `json:"name"`
	Image                 string `json:"image"`
	bundle.VersionRelease `json:",inline"`
	RelatedImages         []string `json:"relatedImages,omitempty"`
}

// BuildPackageData walks FBC content in the given filesystem, groups objects
// by package, and returns a map of package name to PackageData.
func BuildPackageData(ctx context.Context, fsys fs.FS) (map[string]*PackageData, error) {
	var mu sync.Mutex
	result := make(map[string]*PackageData)

	// Side-maps for post-processing, protected by mu.
	type pkgMeta struct {
		Description string
		Icon        *Icon
	}
	pkgMetas := make(map[string]pkgMeta)
	csvMetas := make(map[string]map[string]*csvMetadata) // pkg -> bundle -> csv metadata

	// WalkMetasFS invokes the callback concurrently (one goroutine per
	// file), so all accesses to result must be serialized.
	err := declcfg.WalkMetasFS(ctx, fsys, func(path string, meta *declcfg.Meta, err error) error {
		if err != nil {
			return err
		}

		switch meta.Schema {
		case declcfg.SchemaPackage:
			var pkg declcfg.Package
			if err := json.Unmarshal(meta.Blob, &pkg); err != nil {
				return fmt.Errorf("unmarshaling package from %s: %w", path, err)
			}
			mu.Lock()
			pm := pkgMeta{Description: pkg.Description}
			if pkg.Icon != nil {
				pm.Icon = &Icon{Data: pkg.Icon.Data, MediaType: pkg.Icon.MediaType}
			}
			pkgMetas[pkg.Name] = pm
			mu.Unlock()

		case declcfg.SchemaChannel:
			var ch declcfg.Channel
			if err := json.Unmarshal(meta.Blob, &ch); err != nil {
				return fmt.Errorf("unmarshaling channel from %s: %w", path, err)
			}
			entries := make([]EntryData, len(ch.Entries))
			for i, e := range ch.Entries {
				entries[i] = EntryData{
					Name:      e.Name,
					Replaces:  e.Replaces,
					Skips:     e.Skips,
					SkipRange: e.SkipRange,
				}
			}
			mu.Lock()
			pd := getOrCreate(result, ch.Package)
			pd.Channels = append(pd.Channels, ChannelData{
				Name:    ch.Name,
				Entries: entries,
			})
			mu.Unlock()

		case declcfg.SchemaBundle:
			var b declcfg.Bundle
			if err := json.Unmarshal(meta.Blob, &b); err != nil {
				return fmt.Errorf("unmarshaling bundle from %s: %w", path, err)
			}

			// Deduplicate related images.
			var relatedImages []string
			if len(b.RelatedImages) > 0 {
				seen := make(map[string]struct{})
				for _, ri := range b.RelatedImages {
					if _, ok := seen[ri.Image]; !ok {
						seen[ri.Image] = struct{}{}
						relatedImages = append(relatedImages, ri.Image)
					}
				}
			}

			vr, err := extractBundleVersionRelease(b)
			if err != nil {
				return err
			}

			bd := BundleData{
				Name:           b.Name,
				Image:          b.Image,
				VersionRelease: *vr,
				RelatedImages:  relatedImages,
			}

			// Extract CSV metadata for post-processing.
			var csv *csvMetadata
			for _, p := range b.Properties {
				if p.Type == property.TypeCSVMetadata {
					var m csvMetadata
					if err := json.Unmarshal(p.Value, &m); err == nil {
						csv = &m
					}
					break
				}
			}

			mu.Lock()
			pd := getOrCreate(result, b.Package)
			pd.Bundles = append(pd.Bundles, bd)
			if csv != nil {
				if csvMetas[b.Package] == nil {
					csvMetas[b.Package] = make(map[string]*csvMetadata)
				}
				csvMetas[b.Package][b.Name] = csv
			}
			mu.Unlock()
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking FBC content: %w", err)
	}

	// Post-processing: populate package-level metadata.
	for pkgName, pd := range result {
		// Apply package-level metadata (icon, description).
		if pm, ok := pkgMetas[pkgName]; ok {
			pd.Icon = pm.Icon
			pd.Description = pm.Description
		}

		// Find highest-version bundle.
		var highestBundle string
		var highestVersion *bundle.VersionRelease
		for _, bd := range pd.Bundles {
			if highestVersion == nil || bd.Compare(*highestVersion) > 0 {
				highestVersion = &bd.VersionRelease
				highestBundle = bd.Name
			}
		}

		// Populate from CSV metadata of highest-version bundle.
		if highestBundle != "" {
			if bundleMetas, ok := csvMetas[pkgName]; ok {
				if csv, ok := bundleMetas[highestBundle]; ok {
					pd.DisplayName = csv.DisplayName
					pd.Keywords = csv.Keywords
					if pd.Description == "" {
						pd.Description = csv.Description
					}
				}
			}
		}
	}

	return result, nil
}

// csvMetadata holds the subset of olm.csv.metadata fields we need.
type csvMetadata struct {
	DisplayName string   `json:"displayName,omitempty"`
	Description string   `json:"description,omitempty"`
	Keywords    []string `json:"keywords,omitempty"`
}

func getOrCreate(m map[string]*PackageData, pkg string) *PackageData {
	pd, ok := m[pkg]
	if !ok {
		pd = &PackageData{}
		m[pkg] = pd
	}
	return pd
}

func extractBundleVersionRelease(b declcfg.Bundle) (*bundle.VersionRelease, error) {
	for _, p := range b.Properties {
		if p.Type != property.TypePackage {
			continue
		}
		var pkg property.Package
		if err := json.Unmarshal(p.Value, &pkg); err != nil {
			continue
		}
		if pkg.Release == "" {
			return bundle.NewLegacyRegistryV1VersionRelease(pkg.Version)
		}
		return bundle.NewVersionRelease(pkg.Version, pkg.Release)
	}
	return nil, fmt.Errorf("no valid package property found for %s", b.Name)
}
