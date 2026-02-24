package catalog

import (
	"testing"

	bsemver "github.com/blang/semver/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// addTestCatalog is a helper that adds a catalog with package data to a test DB.
func addTestCatalog(t *testing.T, db *DB, name string, priority int, labels map[string]string, pkgData map[string]*PackageData) {
	t.Helper()
	require.NoError(t, db.AddCatalog(Catalog{
		Name:     name,
		Ref:      "docker://test/" + name + ":latest",
		Priority: priority,
		Labels:   labels,
	}))
	if pkgData != nil {
		require.NoError(t, db.SetPackageData(name, pkgData))
	}
}

// makeTestPackageData creates a simple PackageData with one channel.
func makeTestPackageData(channelName string, bundles []BundleData, entries []EntryData) *PackageData {
	return &PackageData{
		Channels: []ChannelData{{
			Name:    channelName,
			Entries: entries,
		}},
		Bundles: bundles,
	}
}

func TestResolve_PackageNotFound(t *testing.T) {
	db := setupTestDB(t)
	_, err := Resolve(db, "no-such-pkg", ResolveOptions{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestResolve_BasicMatch(t *testing.T) {
	db := setupTestDB(t)
	pkgData := map[string]*PackageData{
		"my-pkg": makeTestPackageData("stable",
			[]BundleData{{Name: "my-pkg.v1.0.0", Image: "reg.io/my-pkg:v1.0.0", VersionRelease: mustVersionRelease("1.0.0", "")}},
			[]EntryData{{Name: "my-pkg.v1.0.0"}},
		),
	}
	addTestCatalog(t, db, "test-cat", 1, nil, pkgData)

	results, err := Resolve(db, "my-pkg", ResolveOptions{})
	require.NoError(t, err)
	require.Len(t, results, 1)

	assert.Equal(t, "test-cat", results[0].CatalogName)
	assert.Equal(t, "my-pkg", results[0].PackageName)
	assert.Equal(t, "my-pkg.v1.0.0", results[0].Name)
	assert.Equal(t, mustVersionRelease("1.0.0", ""), results[0].VersionRelease)
	assert.Equal(t, "reg.io/my-pkg:v1.0.0", results[0].Image)
	assert.Equal(t, []string{"stable"}, results[0].Channels)
}

func TestResolve_CatalogPriority(t *testing.T) {
	db := setupTestDB(t)
	pkgData := map[string]*PackageData{
		"my-pkg": makeTestPackageData("stable",
			[]BundleData{{Name: "my-pkg.v1.0.0", Image: "reg.io/my-pkg:v1.0.0", VersionRelease: mustVersionRelease("1.0.0", "")}},
			[]EntryData{{Name: "my-pkg.v1.0.0"}},
		),
	}
	addTestCatalog(t, db, "low-priority", 1, nil, pkgData)
	addTestCatalog(t, db, "high-priority", 10, nil, pkgData)

	results, err := Resolve(db, "my-pkg", ResolveOptions{})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "high-priority", results[0].CatalogName)
}

func TestResolve_CatalogLabelSelector(t *testing.T) {
	t.Run("Matches", func(t *testing.T) {
		db := setupTestDB(t)
		pkgData := map[string]*PackageData{
			"my-pkg": makeTestPackageData("stable",
				[]BundleData{{Name: "my-pkg.v1.0.0", Image: "reg.io/my-pkg:v1.0.0", VersionRelease: mustVersionRelease("1.0.0", "")}},
				[]EntryData{{Name: "my-pkg.v1.0.0"}},
			),
		}
		addTestCatalog(t, db, "cat1", 1, map[string]string{"env": "prod"}, pkgData)

		results, err := Resolve(db, "my-pkg", ResolveOptions{CatalogLabelSelector: "env=prod"})
		require.NoError(t, err)
		require.Len(t, results, 1)
	})

	t.Run("DoesNotMatch", func(t *testing.T) {
		db := setupTestDB(t)
		pkgData := map[string]*PackageData{
			"my-pkg": makeTestPackageData("stable",
				[]BundleData{{Name: "my-pkg.v1.0.0", Image: "reg.io/my-pkg:v1.0.0", VersionRelease: mustVersionRelease("1.0.0", "")}},
				[]EntryData{{Name: "my-pkg.v1.0.0"}},
			),
		}
		addTestCatalog(t, db, "cat1", 1, map[string]string{"env": "dev"}, pkgData)

		_, err := Resolve(db, "my-pkg", ResolveOptions{CatalogLabelSelector: "env=prod"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestResolve_ChannelFilter(t *testing.T) {
	db := setupTestDB(t)
	pkgData := map[string]*PackageData{
		"my-pkg": {
			Channels: []ChannelData{
				{Name: "stable", Entries: []EntryData{{Name: "my-pkg.v1.0.0"}}},
				{Name: "beta", Entries: []EntryData{{Name: "my-pkg.v1.0.0"}}},
			},
			Bundles: []BundleData{{Name: "my-pkg.v1.0.0", Image: "reg.io/my-pkg:v1.0.0", VersionRelease: mustVersionRelease("1.0.0", "")}},
		},
	}
	addTestCatalog(t, db, "test-cat", 1, nil, pkgData)

	results, err := Resolve(db, "my-pkg", ResolveOptions{Channels: []string{"stable"}})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, []string{"stable"}, results[0].Channels)
}

func TestResolve_ChannelFilter_NoMatch(t *testing.T) {
	db := setupTestDB(t)
	pkgData := map[string]*PackageData{
		"my-pkg": makeTestPackageData("stable",
			[]BundleData{{Name: "my-pkg.v1.0.0", Image: "reg.io/my-pkg:v1.0.0", VersionRelease: mustVersionRelease("1.0.0", "")}},
			[]EntryData{{Name: "my-pkg.v1.0.0"}},
		),
	}
	addTestCatalog(t, db, "test-cat", 1, nil, pkgData)

	_, err := Resolve(db, "my-pkg", ResolveOptions{Channels: []string{"nonexistent"}})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no matching channels")
}

func TestResolve_VersionConstraint(t *testing.T) {
	db := setupTestDB(t)
	pkgData := map[string]*PackageData{
		"my-pkg": makeTestPackageData("stable",
			[]BundleData{
				{Name: "my-pkg.v0.9.0", Image: "reg.io/my-pkg:v0.9.0", VersionRelease: mustVersionRelease("0.9.0", "")},
				{Name: "my-pkg.v1.0.0", Image: "reg.io/my-pkg:v1.0.0", VersionRelease: mustVersionRelease("1.0.0", "")},
				{Name: "my-pkg.v1.5.0", Image: "reg.io/my-pkg:v1.5.0", VersionRelease: mustVersionRelease("1.5.0", "")},
				{Name: "my-pkg.v2.0.0", Image: "reg.io/my-pkg:v2.0.0", VersionRelease: mustVersionRelease("2.0.0", "")},
			},
			[]EntryData{
				{Name: "my-pkg.v0.9.0"},
				{Name: "my-pkg.v1.0.0"},
				{Name: "my-pkg.v1.5.0"},
				{Name: "my-pkg.v2.0.0"},
			},
		),
	}
	addTestCatalog(t, db, "test-cat", 1, nil, pkgData)

	results, err := Resolve(db, "my-pkg", ResolveOptions{Version: ">=1.0.0 <2.0.0"})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, mustVersionRelease("1.5.0", ""), results[0].VersionRelease)
	assert.Equal(t, mustVersionRelease("1.0.0", ""), results[1].VersionRelease)
}

func TestResolve_VersionConstraint_Invalid(t *testing.T) {
	db := setupTestDB(t)
	_, err := Resolve(db, "my-pkg", ResolveOptions{Version: "not-a-constraint!!!"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing version constraint")
}

func TestResolve_InstalledVersion_Replaces(t *testing.T) {
	db := setupTestDB(t)
	pkgData := map[string]*PackageData{
		"my-pkg": makeTestPackageData("stable",
			[]BundleData{
				{Name: "my-pkg.v1.0.0", Image: "reg.io/my-pkg:v1.0.0", VersionRelease: mustVersionRelease("1.0.0", "")},
				{Name: "my-pkg.v2.0.0", Image: "reg.io/my-pkg:v2.0.0", VersionRelease: mustVersionRelease("2.0.0", "")},
			},
			[]EntryData{
				{Name: "my-pkg.v1.0.0"},
				{Name: "my-pkg.v2.0.0", Replaces: "my-pkg.v1.0.0"},
			},
		),
	}
	addTestCatalog(t, db, "test-cat", 1, nil, pkgData)

	results, err := Resolve(db, "my-pkg", ResolveOptions{
		InstalledName:    "my-pkg.v1.0.0",
		InstalledVersion: "1.0.0",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "my-pkg.v2.0.0", results[0].Name)
}

func TestResolve_InstalledVersion_Skips(t *testing.T) {
	db := setupTestDB(t)
	pkgData := map[string]*PackageData{
		"my-pkg": makeTestPackageData("stable",
			[]BundleData{
				{Name: "my-pkg.v1.0.0", Image: "reg.io/my-pkg:v1.0.0", VersionRelease: mustVersionRelease("1.0.0", "")},
				{Name: "my-pkg.v3.0.0", Image: "reg.io/my-pkg:v3.0.0", VersionRelease: mustVersionRelease("3.0.0", "")},
			},
			[]EntryData{
				{Name: "my-pkg.v1.0.0"},
				{Name: "my-pkg.v3.0.0", Skips: []string{"my-pkg.v1.0.0", "my-pkg.v2.0.0"}},
			},
		),
	}
	addTestCatalog(t, db, "test-cat", 1, nil, pkgData)

	results, err := Resolve(db, "my-pkg", ResolveOptions{
		InstalledName:    "my-pkg.v1.0.0",
		InstalledVersion: "1.0.0",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "my-pkg.v3.0.0", results[0].Name)
}

func TestResolve_InstalledVersion_SkipRange(t *testing.T) {
	db := setupTestDB(t)
	pkgData := map[string]*PackageData{
		"my-pkg": makeTestPackageData("stable",
			[]BundleData{
				{Name: "my-pkg.v1.0.0", Image: "reg.io/my-pkg:v1.0.0", VersionRelease: mustVersionRelease("1.0.0", "")},
				{Name: "my-pkg.v2.0.0", Image: "reg.io/my-pkg:v2.0.0", VersionRelease: mustVersionRelease("2.0.0", "")},
			},
			[]EntryData{
				{Name: "my-pkg.v1.0.0"},
				{Name: "my-pkg.v2.0.0", SkipRange: ">=1.0.0 <2.0.0"},
			},
		),
	}
	addTestCatalog(t, db, "test-cat", 1, nil, pkgData)

	results, err := Resolve(db, "my-pkg", ResolveOptions{
		InstalledName:    "my-pkg.v1.0.0",
		InstalledVersion: "1.0.0",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "my-pkg.v2.0.0", results[0].Name)
}

func TestResolve_InstalledVersion_NoSuccessor(t *testing.T) {
	db := setupTestDB(t)
	pkgData := map[string]*PackageData{
		"my-pkg": makeTestPackageData("stable",
			[]BundleData{
				{Name: "my-pkg.v1.0.0", Image: "reg.io/my-pkg:v1.0.0", VersionRelease: mustVersionRelease("1.0.0", "")},
				{Name: "my-pkg.v2.0.0", Image: "reg.io/my-pkg:v2.0.0", VersionRelease: mustVersionRelease("2.0.0", "")},
			},
			[]EntryData{
				{Name: "my-pkg.v1.0.0"},
				{Name: "my-pkg.v2.0.0"}, // no replaces, skips, or skipRange
			},
		),
	}
	addTestCatalog(t, db, "test-cat", 1, nil, pkgData)

	_, err := Resolve(db, "my-pkg", ResolveOptions{
		InstalledName:    "my-pkg.v1.0.0",
		InstalledVersion: "1.0.0",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no bundle found matching constraints")
}

func TestResolve_SortOrder(t *testing.T) {
	db := setupTestDB(t)
	pkgData := map[string]*PackageData{
		"my-pkg": makeTestPackageData("stable",
			[]BundleData{
				{Name: "my-pkg.v1.0.0", Image: "reg.io/my-pkg:v1.0.0", VersionRelease: mustVersionRelease("1.0.0", "")},
				{Name: "my-pkg.v3.0.0", Image: "reg.io/my-pkg:v3.0.0", VersionRelease: mustVersionRelease("3.0.0", "")},
				{Name: "my-pkg.v2.0.0", Image: "reg.io/my-pkg:v2.0.0", VersionRelease: mustVersionRelease("2.0.0", "")},
			},
			[]EntryData{
				{Name: "my-pkg.v1.0.0"},
				{Name: "my-pkg.v3.0.0"},
				{Name: "my-pkg.v2.0.0"},
			},
		),
	}
	addTestCatalog(t, db, "test-cat", 1, nil, pkgData)

	results, err := Resolve(db, "my-pkg", ResolveOptions{})
	require.NoError(t, err)
	require.Len(t, results, 3)
	assert.Equal(t, mustVersionRelease("3.0.0", ""), results[0].VersionRelease)
	assert.Equal(t, mustVersionRelease("2.0.0", ""), results[1].VersionRelease)
	assert.Equal(t, mustVersionRelease("1.0.0", ""), results[2].VersionRelease)
}

func TestResolve_InvalidInstalledVersion(t *testing.T) {
	db := setupTestDB(t)
	_, err := Resolve(db, "my-pkg", ResolveOptions{
		InstalledName:    "my-pkg.v1.0.0",
		InstalledVersion: "not-a-version",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing installed version")
}

func TestResolve_InvalidLabelSelector(t *testing.T) {
	db := setupTestDB(t)
	_, err := Resolve(db, "my-pkg", ResolveOptions{
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
		candidate EntryData
		want      bool
	}{
		{
			name:      "Replaces",
			candidate: EntryData{Name: "pkg.v2.0.0", Replaces: "pkg.v1.0.0"},
			want:      true,
		},
		{
			name:      "Skips",
			candidate: EntryData{Name: "pkg.v3.0.0", Skips: []string{"pkg.v1.0.0"}},
			want:      true,
		},
		{
			name:      "SkipRangeMatch",
			candidate: EntryData{Name: "pkg.v2.0.0", SkipRange: ">=0.9.0 <2.0.0"},
			want:      true,
		},
		{
			name:      "NoMatch",
			candidate: EntryData{Name: "pkg.v2.0.0"},
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
	db := setupTestDB(t)
	pkgData := map[string]*PackageData{
		"my-pkg": makeTestPackageData("stable",
			[]BundleData{{Name: "my-pkg.v1.0.0", Image: "reg.io/my-pkg:v1.0.0", VersionRelease: mustVersionRelease("1.0.0", "")}},
			[]EntryData{{Name: "my-pkg.v1.0.0"}},
		),
	}
	addTestCatalog(t, db, "my-catalog", 1, nil, pkgData)

	// The metadata.name label is automatically added
	results, err := Resolve(db, "my-pkg", ResolveOptions{
		CatalogLabelSelector: "olm.operatorframework.io/metadata.name=my-catalog",
	})
	require.NoError(t, err)
	require.Len(t, results, 1)
}

func TestResolve_BundleInMultipleChannels(t *testing.T) {
	db := setupTestDB(t)
	pkgData := map[string]*PackageData{
		"my-pkg": {
			Channels: []ChannelData{
				{Name: "beta", Entries: []EntryData{{Name: "my-pkg.v1.0.0"}}},
				{Name: "stable", Entries: []EntryData{{Name: "my-pkg.v1.0.0"}}},
			},
			Bundles: []BundleData{{Name: "my-pkg.v1.0.0", Image: "reg.io/my-pkg:v1.0.0", VersionRelease: mustVersionRelease("1.0.0", "")}},
		},
	}
	addTestCatalog(t, db, "test-cat", 1, nil, pkgData)

	results, err := Resolve(db, "my-pkg", ResolveOptions{})
	require.NoError(t, err)
	require.Len(t, results, 1)
	// Channels should be sorted
	assert.Equal(t, []string{"beta", "stable"}, results[0].Channels)
}

func TestResolve_EmptyCatalog(t *testing.T) {
	db := setupTestDB(t)
	pkgData := map[string]*PackageData{
		"my-pkg": makeTestPackageData("stable",
			[]BundleData{{Name: "my-pkg.v1.0.0", Image: "reg.io/my-pkg:v1.0.0", VersionRelease: mustVersionRelease("1.0.0", "")}},
			[]EntryData{{Name: "my-pkg.v1.0.0"}},
		),
	}
	addTestCatalog(t, db, "test-cat", 1, nil, pkgData)

	// Use a version constraint that excludes all bundles
	_, err := Resolve(db, "my-pkg", ResolveOptions{Version: ">=99.0.0"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no bundle found matching constraints")
}

func TestResolve_FlatFBCLayout(t *testing.T) {
	// Test that resolving works when package data was built from a flat FBC layout
	// (i.e., not organized by package subdirectories)
	db := setupTestDB(t)
	pkgData := map[string]*PackageData{
		"pkg-a": makeTestPackageData("stable",
			[]BundleData{{Name: "pkg-a.v1.0.0", Image: "reg.io/pkg-a:v1.0.0", VersionRelease: mustVersionRelease("1.0.0", "")}},
			[]EntryData{{Name: "pkg-a.v1.0.0"}},
		),
		"pkg-b": makeTestPackageData("stable",
			[]BundleData{{Name: "pkg-b.v2.0.0", Image: "reg.io/pkg-b:v2.0.0", VersionRelease: mustVersionRelease("2.0.0", "")}},
			[]EntryData{{Name: "pkg-b.v2.0.0"}},
		),
	}
	addTestCatalog(t, db, "multi-pkg-cat", 1, nil, pkgData)

	resultsA, err := Resolve(db, "pkg-a", ResolveOptions{})
	require.NoError(t, err)
	require.Len(t, resultsA, 1)
	assert.Equal(t, "pkg-a.v1.0.0", resultsA[0].Name)

	resultsB, err := Resolve(db, "pkg-b", ResolveOptions{})
	require.NoError(t, err)
	require.Len(t, resultsB, 1)
	assert.Equal(t, "pkg-b.v2.0.0", resultsB[0].Name)
}

func TestResolve_SortOrder_SameVersionDifferentRelease(t *testing.T) {
	db := setupTestDB(t)

	pkgData := map[string]*PackageData{
		"my-pkg": makeTestPackageData("stable",
			[]BundleData{
				{Name: "my-pkg.v1.0.0-1", Image: "reg.io/my-pkg:v1.0.0-1", VersionRelease: mustVersionRelease("1.0.0", "1")},
				{Name: "my-pkg.v1.0.0-3", Image: "reg.io/my-pkg:v1.0.0-3", VersionRelease: mustVersionRelease("1.0.0", "3")},
				{Name: "my-pkg.v1.0.0-2", Image: "reg.io/my-pkg:v1.0.0-2", VersionRelease: mustVersionRelease("1.0.0", "2")},
			},
			[]EntryData{
				{Name: "my-pkg.v1.0.0-1"},
				{Name: "my-pkg.v1.0.0-3"},
				{Name: "my-pkg.v1.0.0-2"},
			},
		),
	}
	addTestCatalog(t, db, "test-cat", 1, nil, pkgData)

	results, err := Resolve(db, "my-pkg", ResolveOptions{})
	require.NoError(t, err)
	require.Len(t, results, 3)

	// Should be sorted by release descending: 3, 2, 1
	assert.Equal(t, "my-pkg.v1.0.0-3", results[0].Name)
	assert.Equal(t, "my-pkg.v1.0.0-2", results[1].Name)
	assert.Equal(t, "my-pkg.v1.0.0-1", results[2].Name)
}

func TestResolve_VersionConstraint_MatchesBothReleases(t *testing.T) {
	db := setupTestDB(t)

	pkgData := map[string]*PackageData{
		"my-pkg": makeTestPackageData("stable",
			[]BundleData{
				{Name: "my-pkg.v1.0.0-1", Image: "reg.io/my-pkg:v1.0.0-1", VersionRelease: mustVersionRelease("1.0.0", "1")},
				{Name: "my-pkg.v1.0.0-2", Image: "reg.io/my-pkg:v1.0.0-2", VersionRelease: mustVersionRelease("1.0.0", "2")},
				{Name: "my-pkg.v2.0.0-1", Image: "reg.io/my-pkg:v2.0.0-1", VersionRelease: mustVersionRelease("2.0.0", "1")},
			},
			[]EntryData{
				{Name: "my-pkg.v1.0.0-1"},
				{Name: "my-pkg.v1.0.0-2"},
				{Name: "my-pkg.v2.0.0-1"},
			},
		),
	}
	addTestCatalog(t, db, "test-cat", 1, nil, pkgData)

	// Version constraint filters on Version only, so both 1.0.0 releases match
	results, err := Resolve(db, "my-pkg", ResolveOptions{Version: ">=1.0.0 <2.0.0"})
	require.NoError(t, err)
	require.Len(t, results, 2)
	assert.Equal(t, "my-pkg.v1.0.0-2", results[0].Name)
	assert.Equal(t, "my-pkg.v1.0.0-1", results[1].Name)
}
