package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	bsemver "github.com/blang/semver/v4"
	bundlev1 "github.com/joelanford/library-olm/bundle/v1"
	"github.com/joelanford/library-olm/catalog/fbc"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/property"
)

const (
	PropertyDisplayName   = "orb.displayName"
	PropertyDescription   = "orb.description"
	PropertyKeywords      = "orb.keywords"
	PropertyIcon          = "orb.icon"
	PropertyRelatedImages = "orb.relatedImages"
)

type packageExtData struct {
	Description string    `json:"description,omitempty"`
	Icon        *iconData `json:"icon,omitempty"`
}

type bundleExtData struct {
	DisplayName   string   `json:"displayName,omitempty"`
	Description   string   `json:"description,omitempty"`
	Keywords      []string `json:"keywords,omitempty"`
	RelatedImages []string `json:"relatedImages,omitempty"`
}

type iconData struct {
	Data      []byte `json:"data,omitempty"`
	MediaType string `json:"mediaType,omitempty"`
}

type IconValue = iconData

type DisplayMetadataExtension struct{}

var _ fbc.OLMPackageExtension = DisplayMetadataExtension{}

func (DisplayMetadataExtension) OnPackage(pkg declcfg.Package) (any, error) {
	ext := packageExtData{Description: pkg.Description}
	if pkg.Icon != nil {
		ext.Icon = &iconData{Data: pkg.Icon.Data, MediaType: pkg.Icon.MediaType}
	}
	return ext, nil
}

func (DisplayMetadataExtension) OnBundle(b declcfg.Bundle) (any, error) {
	var ext bundleExtData

	for _, p := range b.Properties {
		if p.Type != property.TypeCSVMetadata {
			continue
		}
		var csv struct {
			DisplayName string   `json:"displayName,omitempty"`
			Description string   `json:"description,omitempty"`
			Keywords    []string `json:"keywords,omitempty"`
		}
		if err := json.Unmarshal(p.Value, &csv); err == nil {
			ext.DisplayName = csv.DisplayName
			ext.Description = csv.Description
			ext.Keywords = csv.Keywords
		}
		break
	}

	if len(b.RelatedImages) > 0 {
		seen := make(map[string]struct{})
		for _, ri := range b.RelatedImages {
			if _, ok := seen[ri.Image]; !ok {
				seen[ri.Image] = struct{}{}
				ext.RelatedImages = append(ext.RelatedImages, ri.Image)
			}
		}
	}

	return ext, nil
}

func (DisplayMetadataExtension) OnChannel(declcfg.Channel) (any, error)         { return nil, nil }
func (DisplayMetadataExtension) OnDeprecation(declcfg.Deprecation) (any, error) { return nil, nil }
func (DisplayMetadataExtension) OnOther(declcfg.Meta) (any, error)              { return nil, nil }

func (DisplayMetadataExtension) FinalizePackage(ctx context.Context, pkg fbc.PackageAccessor, w fbc.PropertyWriter) error {
	var pkgExt packageExtData
	if extData, err := pkg.ExtData(); err != nil {
		return fmt.Errorf("reading package ext_data: %w", err)
	} else if extData != nil {
		if err := json.Unmarshal(extData, &pkgExt); err != nil {
			return fmt.Errorf("unmarshaling package ext_data: %w", err)
		}
	}

	type bundleWithExt struct {
		name string
		vr   bundlev1.VersionRelease
		ext  bundleExtData
	}

	var bundles []bundleWithExt
	for b, err := range pkg.Bundles() {
		if err != nil {
			return fmt.Errorf("iterating bundles: %w", err)
		}

		var ext bundleExtData
		if raw := b.ExtData(); raw != nil {
			if err := json.Unmarshal(raw, &ext); err != nil {
				return fmt.Errorf("unmarshaling bundle ext_data for %q: %w", b.Name(), err)
			}
		}

		v, err := bsemver.Parse(b.Version())
		if err != nil {
			return fmt.Errorf("parsing version for bundle %q: %w", b.Name(), err)
		}
		r, err := bundlev1.ParseRelease(b.Release())
		if err != nil {
			return fmt.Errorf("parsing release for bundle %q: %w", b.Name(), err)
		}

		bundles = append(bundles, bundleWithExt{
			name: b.Name(),
			vr:   bundlev1.VersionRelease{Version: v, Release: r},
			ext:  ext,
		})
	}

	if len(bundles) > 0 {
		highest := slices.MaxFunc(bundles, func(a, b bundleWithExt) int {
			return a.vr.Compare(b.vr)
		})

		if highest.ext.DisplayName != "" {
			if err := w.SetGraphProperty(ctx, nil, PropertyDisplayName, highest.ext.DisplayName); err != nil {
				return fmt.Errorf("setting %s: %w", PropertyDisplayName, err)
			}
		}

		description := pkgExt.Description
		if description == "" {
			description = highest.ext.Description
		}
		if description != "" {
			if err := w.SetGraphProperty(ctx, nil, PropertyDescription, description); err != nil {
				return fmt.Errorf("setting %s: %w", PropertyDescription, err)
			}
		}

		if len(highest.ext.Keywords) > 0 {
			keywords := slices.Compact(slices.Sorted(slices.Values(highest.ext.Keywords)))
			if err := w.SetGraphProperty(ctx, nil, PropertyKeywords, keywords); err != nil {
				return fmt.Errorf("setting %s: %w", PropertyKeywords, err)
			}
		}
	}

	if pkgExt.Icon != nil && len(pkgExt.Icon.Data) > 0 {
		if err := w.SetGraphProperty(ctx, nil, PropertyIcon, pkgExt.Icon); err != nil {
			return fmt.Errorf("setting %s: %w", PropertyIcon, err)
		}
	}

	for _, b := range bundles {
		if len(b.ext.RelatedImages) > 0 {
			if err := w.SetBundleProperty(ctx, b.name, PropertyRelatedImages, b.ext.RelatedImages); err != nil {
				return fmt.Errorf("setting %s for bundle %q: %w", PropertyRelatedImages, b.name, err)
			}
		}
	}

	return nil
}
