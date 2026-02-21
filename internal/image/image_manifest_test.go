package image

import (
	"archive/tar"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCombineFilters(t *testing.T) {
	passFilter := func(h *tar.Header) (bool, error) { return true, nil }
	rejectFilter := func(h *tar.Header) (bool, error) { return false, nil }
	errorFilter := func(h *tar.Header) (bool, error) { return false, fmt.Errorf("filter error") }

	t.Run("AllPass", func(t *testing.T) {
		combined := CombineFilters(passFilter, passFilter)
		keep, err := combined(&tar.Header{Name: "test"})
		require.NoError(t, err)
		assert.True(t, keep)
	})

	t.Run("FirstRejects", func(t *testing.T) {
		combined := CombineFilters(rejectFilter, passFilter)
		keep, err := combined(&tar.Header{Name: "test"})
		require.NoError(t, err)
		assert.False(t, keep)
	})

	t.Run("ErrorPropagated", func(t *testing.T) {
		combined := CombineFilters(passFilter, errorFilter)
		_, err := combined(&tar.Header{Name: "test"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "filter error")
	})

	t.Run("EmptyFilters", func(t *testing.T) {
		combined := CombineFilters()
		keep, err := combined(&tar.Header{Name: "test"})
		require.NoError(t, err)
		assert.True(t, keep)
	})
}

func TestOnlyPaths(t *testing.T) {
	t.Run("FileUnderWantedPath", func(t *testing.T) {
		filter := OnlyPaths("manifests")
		keep, err := filter(&tar.Header{Name: "manifests/csv.yaml"})
		require.NoError(t, err)
		assert.True(t, keep)
	})

	t.Run("FileNotUnderWantedPath", func(t *testing.T) {
		filter := OnlyPaths("manifests")
		keep, err := filter(&tar.Header{Name: "other/file.yaml"})
		require.NoError(t, err)
		assert.False(t, keep)
	})

	t.Run("LeadingSlashStripped", func(t *testing.T) {
		filter := OnlyPaths("/manifests")
		keep, err := filter(&tar.Header{Name: "manifests/csv.yaml"})
		require.NoError(t, err)
		assert.True(t, keep)
	})
}

func TestForceOwnershipRWX(t *testing.T) {
	filter := ForceOwnershipRWX()
	h := &tar.Header{
		Name: "test",
		Uid:  9999,
		Gid:  9999,
		Mode: 0444,
	}
	keep, err := filter(h)
	require.NoError(t, err)
	assert.True(t, keep)

	assert.Equal(t, os.Getuid(), h.Uid)
	assert.Equal(t, os.Getgid(), h.Gid)
	assert.Equal(t, int64(0744), h.Mode)
}
