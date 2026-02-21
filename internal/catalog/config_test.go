package catalog

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfig_Add(t *testing.T) {
	t.Run("AddToEmpty", func(t *testing.T) {
		cfg := &Config{}
		err := cfg.Add(Catalog{Name: "cat1", Ref: "ref1"})
		require.NoError(t, err)
		assert.Len(t, cfg.Catalogs, 1)
		assert.Equal(t, "cat1", cfg.Catalogs[0].Name)
	})

	t.Run("AddDuplicate", func(t *testing.T) {
		cfg := &Config{Catalogs: []Catalog{{Name: "cat1", Ref: "ref1"}}}
		err := cfg.Add(Catalog{Name: "cat1", Ref: "ref2"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})

	t.Run("AddMultipleDistinct", func(t *testing.T) {
		cfg := &Config{}
		require.NoError(t, cfg.Add(Catalog{Name: "cat1", Ref: "ref1"}))
		require.NoError(t, cfg.Add(Catalog{Name: "cat2", Ref: "ref2"}))
		assert.Len(t, cfg.Catalogs, 2)
	})
}

func TestConfig_Remove(t *testing.T) {
	t.Run("RemoveExisting", func(t *testing.T) {
		cfg := &Config{Catalogs: []Catalog{
			{Name: "cat1", Ref: "ref1"},
			{Name: "cat2", Ref: "ref2"},
		}}
		removed, err := cfg.Remove("cat1")
		require.NoError(t, err)
		assert.Equal(t, "cat1", removed.Name)
		assert.Len(t, cfg.Catalogs, 1)
		assert.Equal(t, "cat2", cfg.Catalogs[0].Name)
	})

	t.Run("RemoveNonExistent", func(t *testing.T) {
		cfg := &Config{Catalogs: []Catalog{{Name: "cat1"}}}
		_, err := cfg.Remove("missing")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestConfig_Get(t *testing.T) {
	t.Run("GetExisting", func(t *testing.T) {
		cfg := &Config{Catalogs: []Catalog{{Name: "cat1", Ref: "ref1"}}}
		cat, ok := cfg.Get("cat1")
		assert.True(t, ok)
		require.NotNil(t, cat)
		assert.Equal(t, "ref1", cat.Ref)
	})

	t.Run("GetNonExistent", func(t *testing.T) {
		cfg := &Config{Catalogs: []Catalog{{Name: "cat1"}}}
		cat, ok := cfg.Get("missing")
		assert.False(t, ok)
		assert.Nil(t, cat)
	})
}

func TestConfig_SortedCatalogs(t *testing.T) {
	t.Run("PriorityDescending", func(t *testing.T) {
		cfg := &Config{Catalogs: []Catalog{
			{Name: "low", Priority: 1},
			{Name: "high", Priority: 10},
			{Name: "mid", Priority: 5},
		}}
		sorted := cfg.SortedCatalogs()
		require.Len(t, sorted, 3)
		assert.Equal(t, "high", sorted[0].Name)
		assert.Equal(t, "mid", sorted[1].Name)
		assert.Equal(t, "low", sorted[2].Name)
	})

	t.Run("SamePriorityByNameAscending", func(t *testing.T) {
		cfg := &Config{Catalogs: []Catalog{
			{Name: "bravo", Priority: 5},
			{Name: "alpha", Priority: 5},
			{Name: "charlie", Priority: 5},
		}}
		sorted := cfg.SortedCatalogs()
		require.Len(t, sorted, 3)
		assert.Equal(t, "alpha", sorted[0].Name)
		assert.Equal(t, "bravo", sorted[1].Name)
		assert.Equal(t, "charlie", sorted[2].Name)
	})

	t.Run("Empty", func(t *testing.T) {
		cfg := &Config{}
		sorted := cfg.SortedCatalogs()
		assert.Empty(t, sorted)
	})

	t.Run("Single", func(t *testing.T) {
		cfg := &Config{Catalogs: []Catalog{{Name: "only"}}}
		sorted := cfg.SortedCatalogs()
		require.Len(t, sorted, 1)
		assert.Equal(t, "only", sorted[0].Name)
	})
}

func TestLoad(t *testing.T) {
	t.Run("FileNotFound", func(t *testing.T) {
		cfg, err := Load(filepath.Join(t.TempDir(), "nonexistent.json"))
		require.NoError(t, err)
		assert.NotNil(t, cfg)
		assert.Empty(t, cfg.Catalogs)
	})

	t.Run("ValidJSON", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.json")
		data := []byte(`{"catalogs":[{"name":"cat1","ref":"ref1","contentDir":"/tmp","priority":5}]}`)
		require.NoError(t, os.WriteFile(path, data, 0644))

		cfg, err := Load(path)
		require.NoError(t, err)
		require.Len(t, cfg.Catalogs, 1)
		assert.Equal(t, "cat1", cfg.Catalogs[0].Name)
		assert.Equal(t, 5, cfg.Catalogs[0].Priority)
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.json")
		require.NoError(t, os.WriteFile(path, []byte(`{invalid`), 0644))

		_, err := Load(path)
		require.Error(t, err)
	})
}

func TestSave(t *testing.T) {
	t.Run("CreatesParentDirs", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "sub", "dir", "config.json")
		cfg := &Config{Catalogs: []Catalog{{Name: "cat1", Ref: "ref1"}}}
		require.NoError(t, cfg.Save(path))

		_, err := os.Stat(path)
		require.NoError(t, err)
	})

	t.Run("RoundTrip", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "config.json")
		original := &Config{Catalogs: []Catalog{
			{Name: "cat1", Ref: "ref1", ContentDir: "/tmp/cat1", Priority: 10},
			{Name: "cat2", Ref: "ref2", ContentDir: "/tmp/cat2", Priority: 5},
		}}

		require.NoError(t, original.Save(path))
		loaded, err := Load(path)
		require.NoError(t, err)
		assert.Equal(t, original.Catalogs, loaded.Catalogs)
	})
}
