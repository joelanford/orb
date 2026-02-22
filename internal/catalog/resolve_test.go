package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	bsemver "github.com/blang/semver/v4"
	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/operator-framework/operator-registry/alpha/property"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeFBCCatalog writes a catalog.yaml file in the FBC JSON-lines format
// at <dir>/<packageName>/catalog.yaml.
func writeFBCCatalog(t *testing.T, dir, packageName string, fbc declcfg.DeclarativeConfig) {
	t.Helper()
	catalogDir := filepath.Join(dir, packageName)
	require.NoError(t, os.MkdirAll(catalogDir, 0o755))

	var lines []byte
	for i := range fbc.Packages {
		b, err := json.Marshal(fbc.Packages[i])
		require.NoError(t, err)
		lines = append(lines, b...)
		lines = append(lines, '\n')
	}
	for i := range fbc.Channels {
		b, err := json.Marshal(fbc.Channels[i])
		require.NoError(t, err)
		lines = append(lines, b...)
		lines = append(lines, '\n')
	}
	for i := range fbc.Bundles {
		b, err := json.Marshal(fbc.Bundles[i])
		require.NoError(t, err)
		lines = append(lines, b...)
		lines = append(lines, '\n')
	}

	require.NoError(t, os.WriteFile(filepath.Join(catalogDir, "catalog.yaml"), lines, 0o644))
}

// makeFBC creates a simple FBC with the given package name, channel, bundles, and entries.
func makeFBC(packageName, channelName string, bundles []declcfg.Bundle, entries []declcfg.ChannelEntry) declcfg.DeclarativeConfig {
	return declcfg.DeclarativeConfig{
		Packages: []declcfg.Package{
			{Schema: declcfg.SchemaPackage, Name: packageName, DefaultChannel: channelName},
		},
		Channels: []declcfg.Channel{
			{Schema: declcfg.SchemaChannel, Name: channelName, Package: packageName, Entries: entries},
		},
		Bundles: bundles,
	}
}

// makeBundle creates a declcfg.Bundle with an olm.package property.
func makeBundle(name, packageName, version, image string) declcfg.Bundle {
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

func TestResolve_PackageNotFound(t *testing.T) {
	cfg := &Config{}
	_, err := Resolve(cfg, "no-such-pkg", ResolveOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestResolve_BasicMatch(t *testing.T) {
	dir := t.TempDir()
	bundles := []declcfg.Bundle{makeBundle("my-pkg.v1.0.0", "my-pkg", "1.0.0", "reg.io/my-pkg:v1.0.0")}
	entries := []declcfg.ChannelEntry{{Name: "my-pkg.v1.0.0"}}
	fbc := makeFBC("my-pkg", "stable", bundles, entries)
	writeFBCCatalog(t, dir, "my-pkg", fbc)

	cfg := &Config{Catalogs: []Catalog{{Name: "test-cat", ContentDir: dir, Priority: 1}}}
	results, err := Resolve(cfg, "my-pkg", ResolveOptions{})
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "test-cat", results[0].CatalogName)
	assert.Equal(t, "my-pkg", results[0].PackageName)
	assert.Equal(t, "my-pkg.v1.0.0", results[0].BundleName)
	assert.Equal(t, "1.0.0", results[0].Version)
	assert.Equal(t, "reg.io/my-pkg:v1.0.0", results[0].Image)
	assert.Equal(t, []string{"stable"}, results[0].Channels)
}

func TestResolve_CatalogPriority(t *testing.T) {
	dirHigh := t.TempDir()
	dirLow := t.TempDir()

	bundles := []declcfg.Bundle{makeBundle("my-pkg.v1.0.0", "my-pkg", "1.0.0", "reg.io/my-pkg:v1.0.0")}
	entries := []declcfg.ChannelEntry{{Name: "my-pkg.v1.0.0"}}
	fbc := makeFBC("my-pkg", "stable", bundles, entries)

	writeFBCCatalog(t, dirHigh, "my-pkg", fbc)
	writeFBCCatalog(t, dirLow, "my-pkg", fbc)

	cfg := &Config{Catalogs: []Catalog{
		{Name: "low-priority", ContentDir: dirLow, Priority: 1},
		{Name: "high-priority", ContentDir: dirHigh, Priority: 10},
	}}

	results, err := Resolve(cfg, "my-pkg", ResolveOptions{})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "high-priority", results[0].CatalogName)
}

func TestResolve_CatalogLabelSelector(t *testing.T) {
	t.Run("Matches", func(t *testing.T) {
		dir := t.TempDir()
		bundles := []declcfg.Bundle{makeBundle("my-pkg.v1.0.0", "my-pkg", "1.0.0", "reg.io/my-pkg:v1.0.0")}
		entries := []declcfg.ChannelEntry{{Name: "my-pkg.v1.0.0"}}
		writeFBCCatalog(t, dir, "my-pkg", makeFBC("my-pkg", "stable", bundles, entries))

		cfg := &Config{Catalogs: []Catalog{
			{Name: "cat1", ContentDir: dir, Priority: 1, Labels: map[string]string{"env": "prod"}},
		}}

		results, err := Resolve(cfg, "my-pkg", ResolveOptions{CatalogLabelSelector: "env=prod"})
		require.NoError(t, err)
		require.Len(t, results, 1)
	})

	t.Run("DoesNotMatch", func(t *testing.T) {
		dir := t.TempDir()
		bundles := []declcfg.Bundle{makeBundle("my-pkg.v1.0.0", "my-pkg", "1.0.0", "reg.io/my-pkg:v1.0.0")}
		entries := []declcfg.ChannelEntry{{Name: "my-pkg.v1.0.0"}}
		writeFBCCatalog(t, dir, "my-pkg", makeFBC("my-pkg", "stable", bundles, entries))

		cfg := &Config{Catalogs: []Catalog{
			{Name: "cat1", ContentDir: dir, Priority: 1, Labels: map[string]string{"env": "dev"}},
		}}

		_, err := Resolve(cfg, "my-pkg", ResolveOptions{CatalogLabelSelector: "env=prod"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestResolve_ChannelFilter(t *testing.T) {
	dir := t.TempDir()

	b100 := makeBundle("my-pkg.v1.0.0", "my-pkg", "1.0.0", "reg.io/my-pkg:v1.0.0")
	fbc := declcfg.DeclarativeConfig{
		Packages: []declcfg.Package{
			{Schema: declcfg.SchemaPackage, Name: "my-pkg", DefaultChannel: "stable"},
		},
		Channels: []declcfg.Channel{
			{Schema: declcfg.SchemaChannel, Name: "stable", Package: "my-pkg", Entries: []declcfg.ChannelEntry{{Name: "my-pkg.v1.0.0"}}},
			{Schema: declcfg.SchemaChannel, Name: "beta", Package: "my-pkg", Entries: []declcfg.ChannelEntry{{Name: "my-pkg.v1.0.0"}}},
		},
		Bundles: []declcfg.Bundle{b100},
	}
	writeFBCCatalog(t, dir, "my-pkg", fbc)

	cfg := &Config{Catalogs: []Catalog{{Name: "test-cat", ContentDir: dir, Priority: 1}}}

	results, err := Resolve(cfg, "my-pkg", ResolveOptions{Channels: []string{"stable"}})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, []string{"stable"}, results[0].Channels)
}

func TestResolve_ChannelFilter_NoMatch(t *testing.T) {
	dir := t.TempDir()
	bundles := []declcfg.Bundle{makeBundle("my-pkg.v1.0.0", "my-pkg", "1.0.0", "reg.io/my-pkg:v1.0.0")}
	entries := []declcfg.ChannelEntry{{Name: "my-pkg.v1.0.0"}}
	writeFBCCatalog(t, dir, "my-pkg", makeFBC("my-pkg", "stable", bundles, entries))

	cfg := &Config{Catalogs: []Catalog{{Name: "test-cat", ContentDir: dir, Priority: 1}}}

	_, err := Resolve(cfg, "my-pkg", ResolveOptions{Channels: []string{"nonexistent"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no matching channels")
}

func TestResolve_VersionConstraint(t *testing.T) {
	dir := t.TempDir()

	bundles := []declcfg.Bundle{
		makeBundle("my-pkg.v0.9.0", "my-pkg", "0.9.0", "reg.io/my-pkg:v0.9.0"),
		makeBundle("my-pkg.v1.0.0", "my-pkg", "1.0.0", "reg.io/my-pkg:v1.0.0"),
		makeBundle("my-pkg.v1.5.0", "my-pkg", "1.5.0", "reg.io/my-pkg:v1.5.0"),
		makeBundle("my-pkg.v2.0.0", "my-pkg", "2.0.0", "reg.io/my-pkg:v2.0.0"),
	}
	entries := []declcfg.ChannelEntry{
		{Name: "my-pkg.v0.9.0"},
		{Name: "my-pkg.v1.0.0"},
		{Name: "my-pkg.v1.5.0"},
		{Name: "my-pkg.v2.0.0"},
	}
	writeFBCCatalog(t, dir, "my-pkg", makeFBC("my-pkg", "stable", bundles, entries))

	cfg := &Config{Catalogs: []Catalog{{Name: "test-cat", ContentDir: dir, Priority: 1}}}

	results, err := Resolve(cfg, "my-pkg", ResolveOptions{Version: ">=1.0.0 <2.0.0"})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "1.5.0", results[0].Version)
	assert.Equal(t, "1.0.0", results[1].Version)
}

func TestResolve_VersionConstraint_Invalid(t *testing.T) {
	cfg := &Config{}
	_, err := Resolve(cfg, "my-pkg", ResolveOptions{Version: "not-a-constraint!!!"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing version constraint")
}

func TestResolve_InstalledVersion_Replaces(t *testing.T) {
	dir := t.TempDir()

	bundles := []declcfg.Bundle{
		makeBundle("my-pkg.v1.0.0", "my-pkg", "1.0.0", "reg.io/my-pkg:v1.0.0"),
		makeBundle("my-pkg.v2.0.0", "my-pkg", "2.0.0", "reg.io/my-pkg:v2.0.0"),
	}
	entries := []declcfg.ChannelEntry{
		{Name: "my-pkg.v1.0.0"},
		{Name: "my-pkg.v2.0.0", Replaces: "my-pkg.v1.0.0"},
	}
	writeFBCCatalog(t, dir, "my-pkg", makeFBC("my-pkg", "stable", bundles, entries))

	cfg := &Config{Catalogs: []Catalog{{Name: "test-cat", ContentDir: dir, Priority: 1}}}

	results, err := Resolve(cfg, "my-pkg", ResolveOptions{
		InstalledName:    "my-pkg.v1.0.0",
		InstalledVersion: "1.0.0",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "my-pkg.v2.0.0", results[0].BundleName)
}

func TestResolve_InstalledVersion_Skips(t *testing.T) {
	dir := t.TempDir()

	bundles := []declcfg.Bundle{
		makeBundle("my-pkg.v1.0.0", "my-pkg", "1.0.0", "reg.io/my-pkg:v1.0.0"),
		makeBundle("my-pkg.v3.0.0", "my-pkg", "3.0.0", "reg.io/my-pkg:v3.0.0"),
	}
	entries := []declcfg.ChannelEntry{
		{Name: "my-pkg.v1.0.0"},
		{Name: "my-pkg.v3.0.0", Skips: []string{"my-pkg.v1.0.0", "my-pkg.v2.0.0"}},
	}
	writeFBCCatalog(t, dir, "my-pkg", makeFBC("my-pkg", "stable", bundles, entries))

	cfg := &Config{Catalogs: []Catalog{{Name: "test-cat", ContentDir: dir, Priority: 1}}}

	results, err := Resolve(cfg, "my-pkg", ResolveOptions{
		InstalledName:    "my-pkg.v1.0.0",
		InstalledVersion: "1.0.0",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "my-pkg.v3.0.0", results[0].BundleName)
}

func TestResolve_InstalledVersion_SkipRange(t *testing.T) {
	dir := t.TempDir()

	bundles := []declcfg.Bundle{
		makeBundle("my-pkg.v1.0.0", "my-pkg", "1.0.0", "reg.io/my-pkg:v1.0.0"),
		makeBundle("my-pkg.v2.0.0", "my-pkg", "2.0.0", "reg.io/my-pkg:v2.0.0"),
	}
	entries := []declcfg.ChannelEntry{
		{Name: "my-pkg.v1.0.0"},
		{Name: "my-pkg.v2.0.0", SkipRange: ">=1.0.0 <2.0.0"},
	}
	writeFBCCatalog(t, dir, "my-pkg", makeFBC("my-pkg", "stable", bundles, entries))

	cfg := &Config{Catalogs: []Catalog{{Name: "test-cat", ContentDir: dir, Priority: 1}}}

	results, err := Resolve(cfg, "my-pkg", ResolveOptions{
		InstalledName:    "my-pkg.v1.0.0",
		InstalledVersion: "1.0.0",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "my-pkg.v2.0.0", results[0].BundleName)
}

func TestResolve_InstalledVersion_NoSuccessor(t *testing.T) {
	dir := t.TempDir()

	bundles := []declcfg.Bundle{
		makeBundle("my-pkg.v1.0.0", "my-pkg", "1.0.0", "reg.io/my-pkg:v1.0.0"),
		makeBundle("my-pkg.v2.0.0", "my-pkg", "2.0.0", "reg.io/my-pkg:v2.0.0"),
	}
	entries := []declcfg.ChannelEntry{
		{Name: "my-pkg.v1.0.0"},
		{Name: "my-pkg.v2.0.0"}, // no replaces, skips, or skipRange
	}
	writeFBCCatalog(t, dir, "my-pkg", makeFBC("my-pkg", "stable", bundles, entries))

	cfg := &Config{Catalogs: []Catalog{{Name: "test-cat", ContentDir: dir, Priority: 1}}}

	_, err := Resolve(cfg, "my-pkg", ResolveOptions{
		InstalledName:    "my-pkg.v1.0.0",
		InstalledVersion: "1.0.0",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no bundle found matching constraints")
}

func TestResolve_SortOrder(t *testing.T) {
	dir := t.TempDir()

	bundles := []declcfg.Bundle{
		makeBundle("my-pkg.v1.0.0", "my-pkg", "1.0.0", "reg.io/my-pkg:v1.0.0"),
		makeBundle("my-pkg.v3.0.0", "my-pkg", "3.0.0", "reg.io/my-pkg:v3.0.0"),
		makeBundle("my-pkg.v2.0.0", "my-pkg", "2.0.0", "reg.io/my-pkg:v2.0.0"),
	}
	entries := []declcfg.ChannelEntry{
		{Name: "my-pkg.v1.0.0"},
		{Name: "my-pkg.v3.0.0"},
		{Name: "my-pkg.v2.0.0"},
	}
	writeFBCCatalog(t, dir, "my-pkg", makeFBC("my-pkg", "stable", bundles, entries))

	cfg := &Config{Catalogs: []Catalog{{Name: "test-cat", ContentDir: dir, Priority: 1}}}

	results, err := Resolve(cfg, "my-pkg", ResolveOptions{})
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, "3.0.0", results[0].Version)
	assert.Equal(t, "2.0.0", results[1].Version)
	assert.Equal(t, "1.0.0", results[2].Version)
}

func TestResolve_InvalidInstalledVersion(t *testing.T) {
	cfg := &Config{}
	_, err := Resolve(cfg, "my-pkg", ResolveOptions{
		InstalledName:    "my-pkg.v1.0.0",
		InstalledVersion: "not-a-version",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing installed version")
}

func TestResolve_InvalidLabelSelector(t *testing.T) {
	cfg := &Config{}
	_, err := Resolve(cfg, "my-pkg", ResolveOptions{
		CatalogLabelSelector: "!!!invalid",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing catalog label selector")
}

// ---- Tests for unexported helpers ----

func TestIsSuccessor(t *testing.T) {
	installed := bsemver.MustParse("1.0.0")

	tests := []struct {
		name      string
		candidate declcfg.ChannelEntry
		want      bool
	}{
		{
			name:      "Replaces",
			candidate: declcfg.ChannelEntry{Name: "pkg.v2.0.0", Replaces: "pkg.v1.0.0"},
			want:      true,
		},
		{
			name:      "Skips",
			candidate: declcfg.ChannelEntry{Name: "pkg.v3.0.0", Skips: []string{"pkg.v1.0.0"}},
			want:      true,
		},
		{
			name:      "SkipRangeMatch",
			candidate: declcfg.ChannelEntry{Name: "pkg.v2.0.0", SkipRange: ">=0.9.0 <2.0.0"},
			want:      true,
		},
		{
			name:      "NoMatch",
			candidate: declcfg.ChannelEntry{Name: "pkg.v2.0.0"},
			want:      false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isSuccessor(tc.candidate, "pkg.v1.0.0", installed)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestBundleVersion(t *testing.T) {
	t.Run("HasPackageProperty", func(t *testing.T) {
		b := makeBundle("pkg.v1.0.0", "pkg", "1.0.0", "img")
		ver, err := bundleVersion(b)
		require.NoError(t, err)
		assert.Equal(t, "1.0.0", ver)
	})

	t.Run("MissingProperty", func(t *testing.T) {
		b := declcfg.Bundle{Name: "pkg.v1.0.0", Properties: nil}
		_, err := bundleVersion(b)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no olm.package property")
	})

	t.Run("BadJSON", func(t *testing.T) {
		b := declcfg.Bundle{
			Name: "pkg.v1.0.0",
			Properties: []property.Property{
				{Type: property.TypePackage, Value: json.RawMessage(`not-json`)},
			},
		}
		_, err := bundleVersion(b)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unmarshaling package property")
	})
}

func TestResolveResult_ChannelsString(t *testing.T) {
	tests := []struct {
		name     string
		channels []string
		want     string
	}{
		{"Multiple", []string{"alpha", "beta", "stable"}, "alpha,beta,stable"},
		{"Single", []string{"stable"}, "stable"},
		{"Empty", nil, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := ResolveResult{Channels: tc.channels}
			assert.Equal(t, tc.want, r.ChannelsString())
		})
	}
}

func TestResolve_CatalogLabelSelector_MatchByName(t *testing.T) {
	dir := t.TempDir()
	bundles := []declcfg.Bundle{makeBundle("my-pkg.v1.0.0", "my-pkg", "1.0.0", "reg.io/my-pkg:v1.0.0")}
	entries := []declcfg.ChannelEntry{{Name: "my-pkg.v1.0.0"}}
	writeFBCCatalog(t, dir, "my-pkg", makeFBC("my-pkg", "stable", bundles, entries))

	cfg := &Config{Catalogs: []Catalog{
		{Name: "my-catalog", ContentDir: dir, Priority: 1},
	}}

	// The metadata.name label is automatically added
	results, err := Resolve(cfg, "my-pkg", ResolveOptions{
		CatalogLabelSelector: "olm.operatorframework.io/metadata.name=my-catalog",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
}

func TestResolve_BundleInMultipleChannels(t *testing.T) {
	dir := t.TempDir()

	b100 := makeBundle("my-pkg.v1.0.0", "my-pkg", "1.0.0", "reg.io/my-pkg:v1.0.0")
	fbc := declcfg.DeclarativeConfig{
		Packages: []declcfg.Package{
			{Schema: declcfg.SchemaPackage, Name: "my-pkg", DefaultChannel: "stable"},
		},
		Channels: []declcfg.Channel{
			{Schema: declcfg.SchemaChannel, Name: "beta", Package: "my-pkg", Entries: []declcfg.ChannelEntry{{Name: "my-pkg.v1.0.0"}}},
			{Schema: declcfg.SchemaChannel, Name: "stable", Package: "my-pkg", Entries: []declcfg.ChannelEntry{{Name: "my-pkg.v1.0.0"}}},
		},
		Bundles: []declcfg.Bundle{b100},
	}
	writeFBCCatalog(t, dir, "my-pkg", fbc)

	cfg := &Config{Catalogs: []Catalog{{Name: "test-cat", ContentDir: dir, Priority: 1}}}

	results, err := Resolve(cfg, "my-pkg", ResolveOptions{})
	require.NoError(t, err)
	require.Len(t, results, 1)
	// Channels should be sorted
	assert.Equal(t, []string{"beta", "stable"}, results[0].Channels)
}

func TestResolve_CatalogDirNotExist(t *testing.T) {
	cfg := &Config{Catalogs: []Catalog{
		{Name: "cat1", ContentDir: "/nonexistent/path", Priority: 1},
	}}
	_, err := Resolve(cfg, "my-pkg", ResolveOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestResolve_EmptyCatalog(t *testing.T) {
	// Catalog directory exists but has no matching bundles for constraints
	dir := t.TempDir()
	bundles := []declcfg.Bundle{makeBundle("my-pkg.v1.0.0", "my-pkg", "1.0.0", "reg.io/my-pkg:v1.0.0")}
	entries := []declcfg.ChannelEntry{{Name: "my-pkg.v1.0.0"}}
	writeFBCCatalog(t, dir, "my-pkg", makeFBC("my-pkg", "stable", bundles, entries))

	cfg := &Config{Catalogs: []Catalog{{Name: "test-cat", ContentDir: dir, Priority: 1}}}

	// Use a version constraint that excludes all bundles
	_, err := Resolve(cfg, "my-pkg", ResolveOptions{Version: ">=99.0.0"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no bundle found matching constraints")
}
