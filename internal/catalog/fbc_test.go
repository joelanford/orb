package catalog

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/property"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/joelanford/orb/internal/bundle"
)

func writeFBCObjects(t *testing.T, path string, objects ...interface{}) {
	t.Helper()
	dir := filepath.Dir(path)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	var lines []byte
	for _, obj := range objects {
		b, err := json.Marshal(obj)
		require.NoError(t, err)
		lines = append(lines, b...)
		lines = append(lines, '\n')
	}
	require.NoError(t, os.WriteFile(path, lines, 0o644))
}

func makeDeclcfgBundle(name, packageName, version, image string) declcfg.Bundle {
	propVal, _ := json.Marshal(map[string]string{"packageName": packageName, "version": version})
	return declcfg.Bundle{
		Schema:  declcfg.SchemaBundle,
		Name:    name,
		Package: packageName,
		Image:   image,
		Properties: []property.Property{
			{Type: property.TypePackage, Value: propVal},
		},
	}
}

func mustVersionRelease(v, r string) bundle.VersionRelease {
	vr, err := bundle.NewVersionRelease(v, r)
	if err != nil {
		panic(err)
	}
	return *vr
}

func makeDeclcfgBundleWithRelease(name, packageName, version, release, image string) declcfg.Bundle {
	propVal, _ := json.Marshal(map[string]string{"packageName": packageName, "version": version, "release": release})
	return declcfg.Bundle{
		Schema:  declcfg.SchemaBundle,
		Name:    name,
		Package: packageName,
		Image:   image,
		Properties: []property.Property{
			{Type: property.TypePackage, Value: propVal},
		},
	}
}

func makeDeclcfgBundleWithCSV(name, packageName, version, image string, csv csvMetadata) declcfg.Bundle {
	b := makeDeclcfgBundle(name, packageName, version, image)
	csvVal, _ := json.Marshal(csv)
	b.Properties = append(b.Properties, property.Property{Type: property.TypeCSVMetadata, Value: csvVal})
	return b
}

func makeDeclcfgBundleWithRelatedImages(name, packageName, version, image string, relatedImages []declcfg.RelatedImage) declcfg.Bundle {
	b := makeDeclcfgBundle(name, packageName, version, image)
	b.RelatedImages = relatedImages
	return b
}

func TestBuildPackageData_SubdirectoryLayout(t *testing.T) {
	dir := t.TempDir()

	// pkg1 in subdirectory
	writeFBCObjects(t, filepath.Join(dir, "pkg1", "catalog.yaml"),
		declcfg.Package{Schema: declcfg.SchemaPackage, Name: "pkg1", DefaultChannel: "stable"},
		declcfg.Channel{
			Schema:  declcfg.SchemaChannel,
			Name:    "stable",
			Package: "pkg1",
			Entries: []declcfg.ChannelEntry{{Name: "pkg1.v1.0.0"}},
		},
		makeDeclcfgBundle("pkg1.v1.0.0", "pkg1", "1.0.0", "reg.io/pkg1:v1.0.0"),
	)

	// pkg2 in subdirectory
	writeFBCObjects(t, filepath.Join(dir, "pkg2", "catalog.yaml"),
		declcfg.Package{Schema: declcfg.SchemaPackage, Name: "pkg2", DefaultChannel: "stable"},
		declcfg.Channel{
			Schema:  declcfg.SchemaChannel,
			Name:    "stable",
			Package: "pkg2",
			Entries: []declcfg.ChannelEntry{{Name: "pkg2.v2.0.0"}},
		},
		makeDeclcfgBundle("pkg2.v2.0.0", "pkg2", "2.0.0", "reg.io/pkg2:v2.0.0"),
	)

	result, err := BuildPackageData(context.Background(), os.DirFS(dir))
	require.NoError(t, err)
	require.Len(t, result, 2)

	// Verify pkg1
	pd1 := result["pkg1"]
	require.NotNil(t, pd1)
	require.Len(t, pd1.Channels, 1)
	assert.Equal(t, "stable", pd1.Channels[0].Name)
	require.Len(t, pd1.Bundles, 1)
	assert.Equal(t, "pkg1.v1.0.0", pd1.Bundles[0].Name)
	assert.Equal(t, mustVersionRelease("1.0.0", ""), pd1.Bundles[0].VersionRelease)
	assert.Equal(t, "reg.io/pkg1:v1.0.0", pd1.Bundles[0].Image)

	// Verify pkg2
	pd2 := result["pkg2"]
	require.NotNil(t, pd2)
	require.Len(t, pd2.Channels, 1)
	require.Len(t, pd2.Bundles, 1)
	assert.Equal(t, "pkg2.v2.0.0", pd2.Bundles[0].Name)
	assert.Equal(t, mustVersionRelease("2.0.0", ""), pd2.Bundles[0].VersionRelease)
}

func TestBuildPackageData_SingleFlatFile(t *testing.T) {
	dir := t.TempDir()

	// Both packages in a single root file
	writeFBCObjects(t, filepath.Join(dir, "catalog.yaml"),
		declcfg.Package{Schema: declcfg.SchemaPackage, Name: "pkg1", DefaultChannel: "stable"},
		declcfg.Channel{
			Schema:  declcfg.SchemaChannel,
			Name:    "stable",
			Package: "pkg1",
			Entries: []declcfg.ChannelEntry{{Name: "pkg1.v1.0.0"}},
		},
		makeDeclcfgBundle("pkg1.v1.0.0", "pkg1", "1.0.0", "reg.io/pkg1:v1.0.0"),
		declcfg.Package{Schema: declcfg.SchemaPackage, Name: "pkg2", DefaultChannel: "alpha"},
		declcfg.Channel{
			Schema:  declcfg.SchemaChannel,
			Name:    "alpha",
			Package: "pkg2",
			Entries: []declcfg.ChannelEntry{{Name: "pkg2.v0.1.0"}},
		},
		makeDeclcfgBundle("pkg2.v0.1.0", "pkg2", "0.1.0", "reg.io/pkg2:v0.1.0"),
	)

	result, err := BuildPackageData(context.Background(), os.DirFS(dir))
	require.NoError(t, err)
	require.Len(t, result, 2)

	assert.NotNil(t, result["pkg1"])
	assert.NotNil(t, result["pkg2"])
	assert.Equal(t, mustVersionRelease("1.0.0", ""), result["pkg1"].Bundles[0].VersionRelease)
	assert.Equal(t, mustVersionRelease("0.1.0", ""), result["pkg2"].Bundles[0].VersionRelease)
}

func TestBuildPackageData_MixedLayout(t *testing.T) {
	dir := t.TempDir()

	// pkg1 at root
	writeFBCObjects(t, filepath.Join(dir, "root-catalog.yaml"),
		declcfg.Package{Schema: declcfg.SchemaPackage, Name: "pkg1", DefaultChannel: "stable"},
		declcfg.Channel{
			Schema:  declcfg.SchemaChannel,
			Name:    "stable",
			Package: "pkg1",
			Entries: []declcfg.ChannelEntry{{Name: "pkg1.v1.0.0"}},
		},
		makeDeclcfgBundle("pkg1.v1.0.0", "pkg1", "1.0.0", "reg.io/pkg1:v1.0.0"),
	)

	// pkg2 in subdirectory
	writeFBCObjects(t, filepath.Join(dir, "subdir", "pkg2.yaml"),
		declcfg.Package{Schema: declcfg.SchemaPackage, Name: "pkg2", DefaultChannel: "stable"},
		declcfg.Channel{
			Schema:  declcfg.SchemaChannel,
			Name:    "stable",
			Package: "pkg2",
			Entries: []declcfg.ChannelEntry{{Name: "pkg2.v2.0.0"}},
		},
		makeDeclcfgBundle("pkg2.v2.0.0", "pkg2", "2.0.0", "reg.io/pkg2:v2.0.0"),
	)

	result, err := BuildPackageData(context.Background(), os.DirFS(dir))
	require.NoError(t, err)
	require.Len(t, result, 2)

	assert.NotNil(t, result["pkg1"])
	assert.NotNil(t, result["pkg2"])
}

func TestBuildPackageData_MultipleChannels(t *testing.T) {
	dir := t.TempDir()

	writeFBCObjects(t, filepath.Join(dir, "catalog.yaml"),
		declcfg.Package{Schema: declcfg.SchemaPackage, Name: "pkg1", DefaultChannel: "stable"},
		declcfg.Channel{
			Schema:  declcfg.SchemaChannel,
			Name:    "stable",
			Package: "pkg1",
			Entries: []declcfg.ChannelEntry{{Name: "pkg1.v1.0.0"}},
		},
		declcfg.Channel{
			Schema:  declcfg.SchemaChannel,
			Name:    "beta",
			Package: "pkg1",
			Entries: []declcfg.ChannelEntry{{Name: "pkg1.v1.0.0"}, {Name: "pkg1.v2.0.0"}},
		},
		makeDeclcfgBundle("pkg1.v1.0.0", "pkg1", "1.0.0", "reg.io/pkg1:v1.0.0"),
		makeDeclcfgBundle("pkg1.v2.0.0", "pkg1", "2.0.0", "reg.io/pkg1:v2.0.0"),
	)

	result, err := BuildPackageData(context.Background(), os.DirFS(dir))
	require.NoError(t, err)
	require.Len(t, result, 1)

	pd := result["pkg1"]
	require.NotNil(t, pd)
	assert.Len(t, pd.Channels, 2)
	assert.Len(t, pd.Bundles, 2)
}

func TestBuildPackageData_ChannelEntryFields(t *testing.T) {
	dir := t.TempDir()

	writeFBCObjects(t, filepath.Join(dir, "catalog.yaml"),
		declcfg.Package{Schema: declcfg.SchemaPackage, Name: "pkg1", DefaultChannel: "stable"},
		declcfg.Channel{
			Schema:  declcfg.SchemaChannel,
			Name:    "stable",
			Package: "pkg1",
			Entries: []declcfg.ChannelEntry{
				{Name: "pkg1.v1.0.0"},
				{Name: "pkg1.v2.0.0", Replaces: "pkg1.v1.0.0", Skips: []string{"pkg1.v1.1.0"}, SkipRange: ">=1.0.0 <2.0.0"},
			},
		},
		makeDeclcfgBundle("pkg1.v1.0.0", "pkg1", "1.0.0", "reg.io/pkg1:v1.0.0"),
		makeDeclcfgBundle("pkg1.v2.0.0", "pkg1", "2.0.0", "reg.io/pkg1:v2.0.0"),
	)

	result, err := BuildPackageData(context.Background(), os.DirFS(dir))
	require.NoError(t, err)

	pd := result["pkg1"]
	require.NotNil(t, pd)
	require.Len(t, pd.Channels, 1)
	require.Len(t, pd.Channels[0].Entries, 2)

	entry := pd.Channels[0].Entries[1]
	assert.Equal(t, "pkg1.v2.0.0", entry.Name)
	assert.Equal(t, "pkg1.v1.0.0", entry.Replaces)
	assert.Equal(t, []string{"pkg1.v1.1.0"}, entry.Skips)
	assert.Equal(t, ">=1.0.0 <2.0.0", entry.SkipRange)
}

func TestBuildPackageData_PackageDescriptionAndIcon(t *testing.T) {
	dir := t.TempDir()

	writeFBCObjects(t, filepath.Join(dir, "catalog.yaml"),
		declcfg.Package{
			Schema:         declcfg.SchemaPackage,
			Name:           "pkg1",
			DefaultChannel: "stable",
			Description:    "A great package",
			Icon:           &declcfg.Icon{Data: []byte("icondata"), MediaType: "image/png"},
		},
		declcfg.Channel{
			Schema:  declcfg.SchemaChannel,
			Name:    "stable",
			Package: "pkg1",
			Entries: []declcfg.ChannelEntry{{Name: "pkg1.v1.0.0"}},
		},
		makeDeclcfgBundle("pkg1.v1.0.0", "pkg1", "1.0.0", "reg.io/pkg1:v1.0.0"),
	)

	result, err := BuildPackageData(context.Background(), os.DirFS(dir))
	require.NoError(t, err)

	pd := result["pkg1"]
	require.NotNil(t, pd)
	assert.Equal(t, "A great package", pd.Description)
	require.NotNil(t, pd.Icon)
	assert.Equal(t, []byte("icondata"), pd.Icon.Data)
	assert.Equal(t, "image/png", pd.Icon.MediaType)
}

func TestBuildPackageData_CSVMetadataFromHighestVersion(t *testing.T) {
	dir := t.TempDir()

	writeFBCObjects(t, filepath.Join(dir, "catalog.yaml"),
		declcfg.Package{Schema: declcfg.SchemaPackage, Name: "pkg1", DefaultChannel: "stable"},
		declcfg.Channel{
			Schema:  declcfg.SchemaChannel,
			Name:    "stable",
			Package: "pkg1",
			Entries: []declcfg.ChannelEntry{{Name: "pkg1.v1.0.0"}, {Name: "pkg1.v2.0.0"}},
		},
		makeDeclcfgBundleWithCSV("pkg1.v1.0.0", "pkg1", "1.0.0", "reg.io/pkg1:v1.0.0", csvMetadata{
			DisplayName: "Old Name",
			Description: "Old description",
			Keywords:    []string{"old"},
		}),
		makeDeclcfgBundleWithCSV("pkg1.v2.0.0", "pkg1", "2.0.0", "reg.io/pkg1:v2.0.0", csvMetadata{
			DisplayName: "Package One",
			Description: "The best package",
			Keywords:    []string{"database", "nosql"},
		}),
	)

	result, err := BuildPackageData(context.Background(), os.DirFS(dir))
	require.NoError(t, err)

	pd := result["pkg1"]
	require.NotNil(t, pd)
	assert.Equal(t, "Package One", pd.DisplayName)
	assert.Equal(t, "The best package", pd.Description)
	assert.Equal(t, []string{"database", "nosql"}, pd.Keywords)
}

func TestBuildPackageData_PackageDescriptionPrecedence(t *testing.T) {
	dir := t.TempDir()

	writeFBCObjects(t, filepath.Join(dir, "catalog.yaml"),
		declcfg.Package{
			Schema:         declcfg.SchemaPackage,
			Name:           "pkg1",
			DefaultChannel: "stable",
			Description:    "Package-level description",
		},
		declcfg.Channel{
			Schema:  declcfg.SchemaChannel,
			Name:    "stable",
			Package: "pkg1",
			Entries: []declcfg.ChannelEntry{{Name: "pkg1.v1.0.0"}},
		},
		makeDeclcfgBundleWithCSV("pkg1.v1.0.0", "pkg1", "1.0.0", "reg.io/pkg1:v1.0.0", csvMetadata{
			DisplayName: "Package One",
			Description: "CSV description should be ignored",
			Keywords:    []string{"db"},
		}),
	)

	result, err := BuildPackageData(context.Background(), os.DirFS(dir))
	require.NoError(t, err)

	pd := result["pkg1"]
	require.NotNil(t, pd)
	// olm.package description takes precedence over CSV metadata description
	assert.Equal(t, "Package-level description", pd.Description)
	// Other CSV metadata fields are still populated
	assert.Equal(t, "Package One", pd.DisplayName)
	assert.Equal(t, []string{"db"}, pd.Keywords)
}

func TestBuildPackageData_RelatedImagesDeduplicated(t *testing.T) {
	dir := t.TempDir()

	writeFBCObjects(t, filepath.Join(dir, "catalog.yaml"),
		declcfg.Package{Schema: declcfg.SchemaPackage, Name: "pkg1", DefaultChannel: "stable"},
		declcfg.Channel{
			Schema:  declcfg.SchemaChannel,
			Name:    "stable",
			Package: "pkg1",
			Entries: []declcfg.ChannelEntry{{Name: "pkg1.v1.0.0"}},
		},
		makeDeclcfgBundleWithRelatedImages("pkg1.v1.0.0", "pkg1", "1.0.0", "reg.io/pkg1:v1.0.0", []declcfg.RelatedImage{
			{Name: "operator", Image: "reg.io/operator:v1"},
			{Name: "sidecar", Image: "reg.io/sidecar:v1"},
			{Name: "operator-dup", Image: "reg.io/operator:v1"}, // duplicate image
		}),
	)

	result, err := BuildPackageData(context.Background(), os.DirFS(dir))
	require.NoError(t, err)

	pd := result["pkg1"]
	require.NotNil(t, pd)
	require.Len(t, pd.Bundles, 1)
	assert.Equal(t, []string{"reg.io/operator:v1", "reg.io/sidecar:v1"}, pd.Bundles[0].RelatedImages)
}

func TestBuildPackageData_WithRelease(t *testing.T) {
	dir := t.TempDir()

	writeFBCObjects(t, filepath.Join(dir, "catalog.yaml"),
		declcfg.Package{Schema: declcfg.SchemaPackage, Name: "pkg1", DefaultChannel: "stable"},
		declcfg.Channel{
			Schema:  declcfg.SchemaChannel,
			Name:    "stable",
			Package: "pkg1",
			Entries: []declcfg.ChannelEntry{
				{Name: "pkg1.v1.0.0-2"},
				{Name: "pkg1.v1.0.0-3"},
			},
		},
		makeDeclcfgBundleWithRelease("pkg1.v1.0.0-2", "pkg1", "1.0.0", "2", "reg.io/pkg1:v1.0.0-2"),
		makeDeclcfgBundleWithRelease("pkg1.v1.0.0-3", "pkg1", "1.0.0", "3", "reg.io/pkg1:v1.0.0-3"),
	)

	result, err := BuildPackageData(context.Background(), os.DirFS(dir))
	require.NoError(t, err)

	pd := result["pkg1"]
	require.NotNil(t, pd)
	require.Len(t, pd.Bundles, 2)

	// Both bundles should have the same version but different releases.
	bundlesByName := make(map[string]BundleData)
	for _, b := range pd.Bundles {
		bundlesByName[b.Name] = b
	}

	b2 := bundlesByName["pkg1.v1.0.0-2"]
	assert.Equal(t, mustVersionRelease("1.0.0", "2"), b2.VersionRelease)

	b3 := bundlesByName["pkg1.v1.0.0-3"]
	assert.Equal(t, mustVersionRelease("1.0.0", "3"), b3.VersionRelease)
}
