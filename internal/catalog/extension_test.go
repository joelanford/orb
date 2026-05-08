package catalog

import (
	"context"
	"encoding/json"
	"iter"
	"testing"

	"github.com/joelanford/library-olm/catalog/fbc"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/property"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOnPackage(t *testing.T) {
	ext := DisplayMetadataExtension{}

	t.Run("with description and icon", func(t *testing.T) {
		pkg := declcfg.Package{
			Schema:      declcfg.SchemaPackage,
			Name:        "vault",
			Description: "HashiCorp Vault operator",
			Icon:        &declcfg.Icon{Data: []byte("fakeicon"), MediaType: "image/svg+xml"},
		}
		result, err := ext.OnPackage(pkg)
		require.NoError(t, err)

		data, err := json.Marshal(result)
		require.NoError(t, err)
		var got packageExtData
		require.NoError(t, json.Unmarshal(data, &got))

		assert.Equal(t, "HashiCorp Vault operator", got.Description)
		require.NotNil(t, got.Icon)
		assert.Equal(t, []byte("fakeicon"), got.Icon.Data)
		assert.Equal(t, "image/svg+xml", got.Icon.MediaType)
	})

	t.Run("without icon", func(t *testing.T) {
		pkg := declcfg.Package{
			Schema:      declcfg.SchemaPackage,
			Name:        "vault",
			Description: "HashiCorp Vault operator",
		}
		result, err := ext.OnPackage(pkg)
		require.NoError(t, err)

		data, err := json.Marshal(result)
		require.NoError(t, err)
		var got packageExtData
		require.NoError(t, json.Unmarshal(data, &got))

		assert.Equal(t, "HashiCorp Vault operator", got.Description)
		assert.Nil(t, got.Icon)
	})

	t.Run("empty package", func(t *testing.T) {
		result, err := ext.OnPackage(declcfg.Package{})
		require.NoError(t, err)

		data, err := json.Marshal(result)
		require.NoError(t, err)
		var got packageExtData
		require.NoError(t, json.Unmarshal(data, &got))

		assert.Empty(t, got.Description)
		assert.Nil(t, got.Icon)
	})
}

func TestOnBundle(t *testing.T) {
	ext := DisplayMetadataExtension{}

	t.Run("with csv metadata and related images", func(t *testing.T) {
		csvVal, _ := json.Marshal(map[string]any{
			"displayName": "Vault Operator",
			"description": "Manage Vault clusters",
			"keywords":    []string{"vault", "security"},
		})

		b := declcfg.Bundle{
			Schema:  declcfg.SchemaBundle,
			Name:    "vault-operator.v1.0.0",
			Package: "vault",
			Properties: []property.Property{
				{Type: property.TypeCSVMetadata, Value: csvVal},
			},
			RelatedImages: []declcfg.RelatedImage{
				{Image: "reg.io/vault:1.0"},
				{Image: "reg.io/sidecar:2.0"},
			},
		}
		result, err := ext.OnBundle(b)
		require.NoError(t, err)

		data, err := json.Marshal(result)
		require.NoError(t, err)
		var got bundleExtData
		require.NoError(t, json.Unmarshal(data, &got))

		assert.Equal(t, "Vault Operator", got.DisplayName)
		assert.Equal(t, "Manage Vault clusters", got.Description)
		assert.Equal(t, []string{"vault", "security"}, got.Keywords)
		assert.Equal(t, []string{"reg.io/vault:1.0", "reg.io/sidecar:2.0"}, got.RelatedImages)
	})

	t.Run("without csv metadata", func(t *testing.T) {
		b := declcfg.Bundle{
			Schema:  declcfg.SchemaBundle,
			Name:    "vault-operator.v1.0.0",
			Package: "vault",
			RelatedImages: []declcfg.RelatedImage{
				{Image: "reg.io/vault:1.0"},
			},
		}
		result, err := ext.OnBundle(b)
		require.NoError(t, err)

		data, err := json.Marshal(result)
		require.NoError(t, err)
		var got bundleExtData
		require.NoError(t, json.Unmarshal(data, &got))

		assert.Empty(t, got.DisplayName)
		assert.Empty(t, got.Description)
		assert.Nil(t, got.Keywords)
		assert.Equal(t, []string{"reg.io/vault:1.0"}, got.RelatedImages)
	})

	t.Run("deduplicates related images", func(t *testing.T) {
		b := declcfg.Bundle{
			Schema:  declcfg.SchemaBundle,
			Name:    "vault-operator.v1.0.0",
			Package: "vault",
			RelatedImages: []declcfg.RelatedImage{
				{Name: "vault", Image: "reg.io/vault:1.0"},
				{Name: "also-vault", Image: "reg.io/vault:1.0"},
				{Name: "sidecar", Image: "reg.io/sidecar:2.0"},
			},
		}
		result, err := ext.OnBundle(b)
		require.NoError(t, err)

		data, err := json.Marshal(result)
		require.NoError(t, err)
		var got bundleExtData
		require.NoError(t, json.Unmarshal(data, &got))

		assert.Equal(t, []string{"reg.io/vault:1.0", "reg.io/sidecar:2.0"}, got.RelatedImages)
	})

	t.Run("empty bundle", func(t *testing.T) {
		result, err := ext.OnBundle(declcfg.Bundle{})
		require.NoError(t, err)

		data, err := json.Marshal(result)
		require.NoError(t, err)
		var got bundleExtData
		require.NoError(t, json.Unmarshal(data, &got))

		assert.Empty(t, got.DisplayName)
		assert.Nil(t, got.RelatedImages)
	})
}

func TestNoOpCallbacks(t *testing.T) {
	ext := DisplayMetadataExtension{}

	result, err := ext.OnChannel(declcfg.Channel{})
	assert.NoError(t, err)
	assert.Nil(t, result)

	result, err = ext.OnDeprecation(declcfg.Deprecation{})
	assert.NoError(t, err)
	assert.Nil(t, result)

	result, err = ext.OnOther(declcfg.Meta{})
	assert.NoError(t, err)
	assert.Nil(t, result)
}

// --- Test doubles for FinalizePackage ---

type propEntry struct {
	key string
	val any
}

type mockPropertyWriter struct {
	graphProps  []propEntry
	bundleProps map[string][]propEntry
}

func newMockPropertyWriter() *mockPropertyWriter {
	return &mockPropertyWriter{
		bundleProps: make(map[string][]propEntry),
	}
}

func (w *mockPropertyWriter) SetGraphProperty(_ context.Context, _ []string, key string, val any) error {
	w.graphProps = append(w.graphProps, propEntry{key, val})
	return nil
}

func (w *mockPropertyWriter) SetBundleProperty(_ context.Context, bundleName, key string, val any) error {
	w.bundleProps[bundleName] = append(w.bundleProps[bundleName], propEntry{key, val})
	return nil
}

func (w *mockPropertyWriter) graphProp(key string) (any, bool) {
	for _, p := range w.graphProps {
		if p.key == key {
			return p.val, true
		}
	}
	return nil, false
}

func (w *mockPropertyWriter) bundleProp(bundle, key string) (any, bool) {
	for _, p := range w.bundleProps[bundle] {
		if p.key == key {
			return p.val, true
		}
	}
	return nil, false
}

type mockBundleAccessor struct {
	name    string
	pkg     string
	version string
	release string
	image   string
	extData json.RawMessage
}

func (b mockBundleAccessor) Name() string             { return b.name }
func (b mockBundleAccessor) Package() string          { return b.pkg }
func (b mockBundleAccessor) Version() string          { return b.version }
func (b mockBundleAccessor) Release() string          { return b.release }
func (b mockBundleAccessor) Image() string            { return b.image }
func (b mockBundleAccessor) ExtData() json.RawMessage { return b.extData }

type mockPackageAccessor struct {
	name    string
	extData json.RawMessage
	extErr  error
	bundles []mockBundleAccessor
}

func (p *mockPackageAccessor) Name() string { return p.name }

func (p *mockPackageAccessor) ExtData() (json.RawMessage, error) {
	return p.extData, p.extErr
}

func (p *mockPackageAccessor) Bundles() iter.Seq2[fbc.BundleAccessor, error] {
	return func(yield func(fbc.BundleAccessor, error) bool) {
		for _, b := range p.bundles {
			if !yield(b, nil) {
				return
			}
		}
	}
}

func (p *mockPackageAccessor) Channels() iter.Seq2[fbc.ChannelAccessor, error] {
	return func(yield func(fbc.ChannelAccessor, error) bool) {}
}

func (p *mockPackageAccessor) Deprecations() iter.Seq2[fbc.DeprecationAccessor, error] {
	return func(yield func(fbc.DeprecationAccessor, error) bool) {}
}

func (p *mockPackageAccessor) Others() iter.Seq2[fbc.OtherAccessor, error] {
	return func(yield func(fbc.OtherAccessor, error) bool) {}
}

func mustMarshal(t *testing.T, v any) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}

func TestFinalizePackage(t *testing.T) {
	ext := DisplayMetadataExtension{}
	ctx := context.Background()

	t.Run("highest version bundle provides display metadata", func(t *testing.T) {
		pkg := &mockPackageAccessor{
			name:    "vault",
			extData: mustMarshal(t, packageExtData{Description: "Package-level description"}),
			bundles: []mockBundleAccessor{
				{
					name:    "vault.v1.0.0",
					version: "1.0.0",
					extData: mustMarshal(t, bundleExtData{
						DisplayName: "Vault v1",
						Description: "Old description",
						Keywords:    []string{"security"},
					}),
				},
				{
					name:    "vault.v2.0.0",
					version: "2.0.0",
					extData: mustMarshal(t, bundleExtData{
						DisplayName:   "Vault Operator",
						Description:   "Bundle description",
						Keywords:      []string{"vault", "security"},
						RelatedImages: []string{"reg.io/vault:2.0"},
					}),
				},
			},
		}

		w := newMockPropertyWriter()
		require.NoError(t, ext.FinalizePackage(ctx, pkg, w))

		val, ok := w.graphProp(PropertyDisplayName)
		require.True(t, ok)
		assert.Equal(t, "Vault Operator", val)

		val, ok = w.graphProp(PropertyDescription)
		require.True(t, ok)
		assert.Equal(t, "Package-level description", val)

		val, ok = w.graphProp(PropertyKeywords)
		require.True(t, ok)
		assert.Equal(t, []string{"security", "vault"}, val)

		val, ok = w.bundleProp("vault.v2.0.0", PropertyRelatedImages)
		require.True(t, ok)
		assert.Equal(t, []string{"reg.io/vault:2.0"}, val)

		_, ok = w.bundleProp("vault.v1.0.0", PropertyRelatedImages)
		assert.False(t, ok)
	})

	t.Run("description falls back to bundle when package is empty", func(t *testing.T) {
		pkg := &mockPackageAccessor{
			name:    "vault",
			extData: mustMarshal(t, packageExtData{}),
			bundles: []mockBundleAccessor{
				{
					name:    "vault.v1.0.0",
					version: "1.0.0",
					extData: mustMarshal(t, bundleExtData{
						DisplayName: "Vault",
						Description: "Bundle fallback description",
					}),
				},
			},
		}

		w := newMockPropertyWriter()
		require.NoError(t, ext.FinalizePackage(ctx, pkg, w))

		val, ok := w.graphProp(PropertyDescription)
		require.True(t, ok)
		assert.Equal(t, "Bundle fallback description", val)
	})

	t.Run("icon from package ext data", func(t *testing.T) {
		pkg := &mockPackageAccessor{
			name: "vault",
			extData: mustMarshal(t, packageExtData{
				Icon: &iconData{Data: []byte("png-data"), MediaType: "image/png"},
			}),
			bundles: []mockBundleAccessor{
				{
					name:    "vault.v1.0.0",
					version: "1.0.0",
				},
			},
		}

		w := newMockPropertyWriter()
		require.NoError(t, ext.FinalizePackage(ctx, pkg, w))

		val, ok := w.graphProp(PropertyIcon)
		require.True(t, ok)
		icon, ok := val.(*iconData)
		require.True(t, ok)
		assert.Equal(t, []byte("png-data"), icon.Data)
		assert.Equal(t, "image/png", icon.MediaType)
	})

	t.Run("release breaks version ties", func(t *testing.T) {
		pkg := &mockPackageAccessor{
			name: "vault",
			bundles: []mockBundleAccessor{
				{
					name:    "vault.v1.0.0_rc.1",
					version: "1.0.0",
					release: "rc.1",
					extData: mustMarshal(t, bundleExtData{DisplayName: "RC"}),
				},
				{
					name:    "vault.v1.0.0_rc.2",
					version: "1.0.0",
					release: "rc.2",
					extData: mustMarshal(t, bundleExtData{DisplayName: "RC2"}),
				},
			},
		}

		w := newMockPropertyWriter()
		require.NoError(t, ext.FinalizePackage(ctx, pkg, w))

		val, ok := w.graphProp(PropertyDisplayName)
		require.True(t, ok)
		assert.Equal(t, "RC2", val)
	})

	t.Run("no bundles produces no properties", func(t *testing.T) {
		pkg := &mockPackageAccessor{
			name:    "empty",
			extData: mustMarshal(t, packageExtData{Description: "Has description but no bundles"}),
		}

		w := newMockPropertyWriter()
		require.NoError(t, ext.FinalizePackage(ctx, pkg, w))

		_, ok := w.graphProp(PropertyDisplayName)
		assert.False(t, ok)
		_, ok = w.graphProp(PropertyDescription)
		assert.False(t, ok)
		_, ok = w.graphProp(PropertyKeywords)
		assert.False(t, ok)
		_, ok = w.graphProp(PropertyIcon)
		assert.False(t, ok)
	})

	t.Run("nil package ext data", func(t *testing.T) {
		pkg := &mockPackageAccessor{
			name: "vault",
			bundles: []mockBundleAccessor{
				{
					name:    "vault.v1.0.0",
					version: "1.0.0",
					extData: mustMarshal(t, bundleExtData{
						DisplayName: "Vault",
						Description: "From bundle",
					}),
				},
			},
		}

		w := newMockPropertyWriter()
		require.NoError(t, ext.FinalizePackage(ctx, pkg, w))

		val, ok := w.graphProp(PropertyDisplayName)
		require.True(t, ok)
		assert.Equal(t, "Vault", val)

		val, ok = w.graphProp(PropertyDescription)
		require.True(t, ok)
		assert.Equal(t, "From bundle", val)
	})

	t.Run("empty display name and keywords are omitted", func(t *testing.T) {
		pkg := &mockPackageAccessor{
			name: "vault",
			bundles: []mockBundleAccessor{
				{
					name:    "vault.v1.0.0",
					version: "1.0.0",
					extData: mustMarshal(t, bundleExtData{Description: "Only desc"}),
				},
			},
		}

		w := newMockPropertyWriter()
		require.NoError(t, ext.FinalizePackage(ctx, pkg, w))

		_, ok := w.graphProp(PropertyDisplayName)
		assert.False(t, ok)
		_, ok = w.graphProp(PropertyKeywords)
		assert.False(t, ok)

		val, ok := w.graphProp(PropertyDescription)
		require.True(t, ok)
		assert.Equal(t, "Only desc", val)
	})

	t.Run("keywords are sorted and deduplicated", func(t *testing.T) {
		pkg := &mockPackageAccessor{
			name: "vault",
			bundles: []mockBundleAccessor{
				{
					name:    "vault.v1.0.0",
					version: "1.0.0",
					extData: mustMarshal(t, bundleExtData{
						Keywords: []string{"zebra", "apple", "apple", "banana"},
					}),
				},
			},
		}

		w := newMockPropertyWriter()
		require.NoError(t, ext.FinalizePackage(ctx, pkg, w))

		val, ok := w.graphProp(PropertyKeywords)
		require.True(t, ok)
		assert.Equal(t, []string{"apple", "banana", "zebra"}, val)
	})

	t.Run("multiple bundles with related images", func(t *testing.T) {
		pkg := &mockPackageAccessor{
			name: "vault",
			bundles: []mockBundleAccessor{
				{
					name:    "vault.v1.0.0",
					version: "1.0.0",
					extData: mustMarshal(t, bundleExtData{
						RelatedImages: []string{"reg.io/a:1"},
					}),
				},
				{
					name:    "vault.v2.0.0",
					version: "2.0.0",
					extData: mustMarshal(t, bundleExtData{
						RelatedImages: []string{"reg.io/a:2", "reg.io/b:2"},
					}),
				},
			},
		}

		w := newMockPropertyWriter()
		require.NoError(t, ext.FinalizePackage(ctx, pkg, w))

		val, ok := w.bundleProp("vault.v1.0.0", PropertyRelatedImages)
		require.True(t, ok)
		assert.Equal(t, []string{"reg.io/a:1"}, val)

		val, ok = w.bundleProp("vault.v2.0.0", PropertyRelatedImages)
		require.True(t, ok)
		assert.Equal(t, []string{"reg.io/a:2", "reg.io/b:2"}, val)
	})
}
