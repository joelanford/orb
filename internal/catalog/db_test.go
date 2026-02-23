package catalog

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/operator-framework/operator-registry/alpha/declcfg"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := OpenDB(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func TestDB_AddCatalog(t *testing.T) {
	t.Run("AddToEmpty", func(t *testing.T) {
		db := setupTestDB(t)
		err := db.AddCatalog(Catalog{Name: "cat1", Ref: "ref1"})
		require.NoError(t, err)

		cat, err := db.GetCatalog("cat1")
		require.NoError(t, err)
		require.NotNil(t, cat)
		assert.Equal(t, "cat1", cat.Name)
		assert.Equal(t, "ref1", cat.Ref)
	})

	t.Run("AddDuplicate", func(t *testing.T) {
		db := setupTestDB(t)
		require.NoError(t, db.AddCatalog(Catalog{Name: "cat1", Ref: "ref1"}))
		err := db.AddCatalog(Catalog{Name: "cat1", Ref: "ref2"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("AddMultipleDistinct", func(t *testing.T) {
		db := setupTestDB(t)
		require.NoError(t, db.AddCatalog(Catalog{Name: "cat1", Ref: "ref1"}))
		require.NoError(t, db.AddCatalog(Catalog{Name: "cat2", Ref: "ref2"}))

		catalogs, err := db.SortedCatalogs()
		require.NoError(t, err)
		assert.Len(t, catalogs, 2)
	})

	t.Run("WithLabels", func(t *testing.T) {
		db := setupTestDB(t)
		err := db.AddCatalog(Catalog{
			Name:   "cat1",
			Ref:    "ref1",
			Labels: map[string]string{"env": "prod", "tier": "community"},
		})
		require.NoError(t, err)

		cat, err := db.GetCatalog("cat1")
		require.NoError(t, err)
		require.NotNil(t, cat)
		assert.Equal(t, map[string]string{"env": "prod", "tier": "community"}, cat.Labels)
	})

	t.Run("WithDigest", func(t *testing.T) {
		db := setupTestDB(t)
		err := db.AddCatalog(Catalog{Name: "cat1", Ref: "ref1", Digest: "sha256:abc123"})
		require.NoError(t, err)

		cat, err := db.GetCatalog("cat1")
		require.NoError(t, err)
		require.NotNil(t, cat)
		assert.Equal(t, "sha256:abc123", cat.Digest)
	})
}

func TestDB_RemoveCatalog(t *testing.T) {
	t.Run("RemoveExisting", func(t *testing.T) {
		db := setupTestDB(t)
		require.NoError(t, db.AddCatalog(Catalog{Name: "cat1", Ref: "ref1"}))
		require.NoError(t, db.AddCatalog(Catalog{Name: "cat2", Ref: "ref2"}))

		removed, err := db.RemoveCatalog("cat1")
		require.NoError(t, err)
		assert.Equal(t, "cat1", removed.Name)

		catalogs, err := db.SortedCatalogs()
		require.NoError(t, err)
		assert.Len(t, catalogs, 1)
		assert.Equal(t, "cat2", catalogs[0].Name)
	})

	t.Run("RemoveNonExistent", func(t *testing.T) {
		db := setupTestDB(t)
		_, err := db.RemoveCatalog("missing")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestDB_GetCatalog(t *testing.T) {
	t.Run("GetExisting", func(t *testing.T) {
		db := setupTestDB(t)
		require.NoError(t, db.AddCatalog(Catalog{Name: "cat1", Ref: "ref1", Priority: 5}))

		cat, err := db.GetCatalog("cat1")
		require.NoError(t, err)
		require.NotNil(t, cat)
		assert.Equal(t, "ref1", cat.Ref)
		assert.Equal(t, 5, cat.Priority)
	})

	t.Run("GetNonExistent", func(t *testing.T) {
		db := setupTestDB(t)
		cat, err := db.GetCatalog("missing")
		require.NoError(t, err)
		assert.Nil(t, cat)
	})
}

func TestDB_UpdateCatalog(t *testing.T) {
	t.Run("UpdateExisting", func(t *testing.T) {
		db := setupTestDB(t)
		require.NoError(t, db.AddCatalog(Catalog{Name: "cat1", Ref: "ref1", Priority: 1}))

		err := db.UpdateCatalog("cat1", func(cat *Catalog) {
			cat.Priority = 10
			cat.Digest = "sha256:updated"
		})
		require.NoError(t, err)

		cat, err := db.GetCatalog("cat1")
		require.NoError(t, err)
		assert.Equal(t, 10, cat.Priority)
		assert.Equal(t, "sha256:updated", cat.Digest)
	})

	t.Run("UpdateNonExistent", func(t *testing.T) {
		db := setupTestDB(t)
		err := db.UpdateCatalog("missing", func(cat *Catalog) {
			cat.Priority = 10
		})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("UpdateLabels", func(t *testing.T) {
		db := setupTestDB(t)
		require.NoError(t, db.AddCatalog(Catalog{
			Name:   "cat1",
			Ref:    "ref1",
			Labels: map[string]string{"env": "dev"},
		}))

		err := db.UpdateCatalog("cat1", func(cat *Catalog) {
			cat.Labels["env"] = "prod"
			cat.Labels["tier"] = "community"
		})
		require.NoError(t, err)

		cat, err := db.GetCatalog("cat1")
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"env": "prod", "tier": "community"}, cat.Labels)
	})
}

func TestDB_SortedCatalogs(t *testing.T) {
	t.Run("PriorityDescending", func(t *testing.T) {
		db := setupTestDB(t)
		require.NoError(t, db.AddCatalog(Catalog{Name: "low", Ref: "r", Priority: 1}))
		require.NoError(t, db.AddCatalog(Catalog{Name: "high", Ref: "r", Priority: 10}))
		require.NoError(t, db.AddCatalog(Catalog{Name: "mid", Ref: "r", Priority: 5}))

		sorted, err := db.SortedCatalogs()
		require.NoError(t, err)
		require.Len(t, sorted, 3)
		assert.Equal(t, "high", sorted[0].Name)
		assert.Equal(t, "mid", sorted[1].Name)
		assert.Equal(t, "low", sorted[2].Name)
	})

	t.Run("SamePriorityByNameAscending", func(t *testing.T) {
		db := setupTestDB(t)
		require.NoError(t, db.AddCatalog(Catalog{Name: "bravo", Ref: "r", Priority: 5}))
		require.NoError(t, db.AddCatalog(Catalog{Name: "alpha", Ref: "r", Priority: 5}))
		require.NoError(t, db.AddCatalog(Catalog{Name: "charlie", Ref: "r", Priority: 5}))

		sorted, err := db.SortedCatalogs()
		require.NoError(t, err)
		require.Len(t, sorted, 3)
		assert.Equal(t, "alpha", sorted[0].Name)
		assert.Equal(t, "bravo", sorted[1].Name)
		assert.Equal(t, "charlie", sorted[2].Name)
	})

	t.Run("Empty", func(t *testing.T) {
		db := setupTestDB(t)
		sorted, err := db.SortedCatalogs()
		require.NoError(t, err)
		assert.Empty(t, sorted)
	})
}

func TestDB_CascadeDelete(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AddCatalog(Catalog{Name: "cat1", Ref: "ref1"}))
	require.NoError(t, db.SetPackageData("cat1", map[string]*PackageData{
		"pkg1": {
			Channels: []ChannelData{{Name: "stable", Entries: []EntryData{{Name: "pkg1.v1.0.0"}}}},
			Bundles:  []BundleData{{Name: "pkg1.v1.0.0", Image: "img", Version: "1.0.0"}},
		},
	}))

	// Verify package data exists
	pd, err := db.GetPackageData("cat1", "pkg1")
	require.NoError(t, err)
	require.NotNil(t, pd)

	// Remove catalog - should cascade delete packages
	_, err = db.RemoveCatalog("cat1")
	require.NoError(t, err)

	pd, err = db.GetPackageData("cat1", "pkg1")
	require.NoError(t, err)
	assert.Nil(t, pd)
}

func TestDB_PackageData(t *testing.T) {
	t.Run("RoundTrip", func(t *testing.T) {
		db := setupTestDB(t)
		require.NoError(t, db.AddCatalog(Catalog{Name: "cat1", Ref: "ref1"}))

		input := map[string]*PackageData{
			"pkg1": {
				DisplayName: "Package One",
				Description: "A test package",
				Keywords:    []string{"database", "nosql"},
				Icon:        &Icon{Data: []byte("icondata"), MediaType: "image/svg+xml"},
				Channels: []ChannelData{
					{
						Name: "stable",
						Entries: []EntryData{
							{Name: "pkg1.v1.0.0"},
							{Name: "pkg1.v2.0.0", Replaces: "pkg1.v1.0.0", SkipRange: ">=1.0.0 <2.0.0"},
						},
					},
					{
						Name: "beta",
						Entries: []EntryData{
							{Name: "pkg1.v1.0.0"},
							{Name: "pkg1.v3.0.0", Skips: []string{"pkg1.v1.0.0", "pkg1.v2.0.0"}},
						},
					},
				},
				Bundles: []BundleData{
					{Name: "pkg1.v1.0.0", Image: "reg.io/pkg1:v1.0.0", Version: "1.0.0"},
					{Name: "pkg1.v2.0.0", Image: "reg.io/pkg1:v2.0.0", Version: "2.0.0", RelatedImages: []string{"reg.io/sidecar:v2"}},
					{Name: "pkg1.v3.0.0", Image: "reg.io/pkg1:v3.0.0", Version: "3.0.0", RelatedImages: []string{"reg.io/init:v3", "reg.io/sidecar:v3"}},
				},
			},
		}

		require.NoError(t, db.SetPackageData("cat1", input))

		pd, err := db.GetPackageData("cat1", "pkg1")
		require.NoError(t, err)
		require.NotNil(t, pd)
		assert.Equal(t, input["pkg1"], pd)
	})

	t.Run("GetNonExistent", func(t *testing.T) {
		db := setupTestDB(t)
		require.NoError(t, db.AddCatalog(Catalog{Name: "cat1", Ref: "ref1"}))

		pd, err := db.GetPackageData("cat1", "no-such-pkg")
		require.NoError(t, err)
		assert.Nil(t, pd)
	})

	t.Run("ReplaceExisting", func(t *testing.T) {
		db := setupTestDB(t)
		require.NoError(t, db.AddCatalog(Catalog{Name: "cat1", Ref: "ref1"}))

		first := map[string]*PackageData{
			"pkg1": {
				Bundles: []BundleData{{Name: "pkg1.v1.0.0", Image: "img1", Version: "1.0.0"}},
			},
		}
		require.NoError(t, db.SetPackageData("cat1", first))

		second := map[string]*PackageData{
			"pkg1": {
				Bundles: []BundleData{{Name: "pkg1.v2.0.0", Image: "img2", Version: "2.0.0"}},
			},
		}
		require.NoError(t, db.SetPackageData("cat1", second))

		pd, err := db.GetPackageData("cat1", "pkg1")
		require.NoError(t, err)
		require.NotNil(t, pd)
		require.Len(t, pd.Bundles, 1)
		assert.Equal(t, "pkg1.v2.0.0", pd.Bundles[0].Name)
	})

	t.Run("MultiplePackages", func(t *testing.T) {
		db := setupTestDB(t)
		require.NoError(t, db.AddCatalog(Catalog{Name: "cat1", Ref: "ref1"}))

		pkgs := map[string]*PackageData{
			"pkg1": {Bundles: []BundleData{{Name: "pkg1.v1.0.0", Image: "img1", Version: "1.0.0"}}},
			"pkg2": {Bundles: []BundleData{{Name: "pkg2.v1.0.0", Image: "img2", Version: "1.0.0"}}},
		}
		require.NoError(t, db.SetPackageData("cat1", pkgs))

		pd1, err := db.GetPackageData("cat1", "pkg1")
		require.NoError(t, err)
		require.NotNil(t, pd1)

		pd2, err := db.GetPackageData("cat1", "pkg2")
		require.NoError(t, err)
		require.NotNil(t, pd2)
	})
}

// Verify BuildPackageData + DB integration for resolve.
func TestResolve_AfterBuildPackageData(t *testing.T) {
	dir := t.TempDir()

	// Create flat FBC with two packages
	pkg1 := declcfg.Package{Schema: declcfg.SchemaPackage, Name: "vault", DefaultChannel: "stable"}
	ch1 := declcfg.Channel{
		Schema: declcfg.SchemaChannel, Name: "stable", Package: "vault",
		Entries: []declcfg.ChannelEntry{{Name: "vault.v1.0.0"}},
	}
	b1 := makeDeclcfgBundle("vault.v1.0.0", "vault", "1.0.0", "reg.io/vault:v1.0.0")

	pkg2 := declcfg.Package{Schema: declcfg.SchemaPackage, Name: "redis", DefaultChannel: "stable"}
	ch2 := declcfg.Channel{
		Schema: declcfg.SchemaChannel, Name: "stable", Package: "redis",
		Entries: []declcfg.ChannelEntry{{Name: "redis.v2.0.0"}},
	}
	b2 := makeDeclcfgBundle("redis.v2.0.0", "redis", "2.0.0", "reg.io/redis:v2.0.0")

	writeFBCObjects(t, filepath.Join(dir, "catalog.yaml"), pkg1, ch1, b1, pkg2, ch2, b2)

	pkgData, err := BuildPackageData(context.Background(), os.DirFS(dir))
	require.NoError(t, err)
	require.Len(t, pkgData, 2)

	db := setupTestDB(t)
	require.NoError(t, db.AddCatalog(Catalog{Name: "test-cat", Ref: "docker://test:latest", Priority: 1}))
	require.NoError(t, db.SetPackageData("test-cat", pkgData))

	results, err := Resolve(db, "vault", ResolveOptions{})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "vault.v1.0.0", results[0].BundleName)

	results, err = Resolve(db, "redis", ResolveOptions{})
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "redis.v2.0.0", results[0].BundleName)
}
