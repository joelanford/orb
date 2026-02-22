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
	passFilter := func(_ *tar.Header) (bool, error) { return true, nil }
	rejectFilter := func(_ *tar.Header) (bool, error) { return false, nil }
	errorFilter := func(_ *tar.Header) (bool, error) { return false, fmt.Errorf("filter error") }

	t.Run("AllPass", func(t *testing.T) {
		combined := CombineFilters(passFilter, passFilter) //nolint:gocritic // intentional duplicate for testing
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

func TestRewritePath(t *testing.T) {
	tests := []struct {
		name     string
		srcPath  string
		destPath string
		input    string
		expected string
	}{
		{
			name:     "RewriteUnderSrc",
			srcPath:  "/configs",
			destPath: "/",
			input:    "configs/foo",
			expected: "foo",
		},
		{
			name:     "RewriteNestedPath",
			srcPath:  "/configs",
			destPath: "/",
			input:    "configs/bar/baz",
			expected: "bar/baz",
		},
		{
			name:     "RewriteSrcDirItself",
			srcPath:  "/configs",
			destPath: "/",
			input:    "configs",
			expected: ".",
		},
		{
			name:     "RewriteToDifferentDest",
			srcPath:  "/configs",
			destPath: "/output",
			input:    "configs/foo",
			expected: "output/foo",
		},
		{
			name:     "EntryOutsideSrcUnchanged",
			srcPath:  "/configs",
			destPath: "/",
			input:    "other/file.yaml",
			expected: "other/file.yaml",
		},
		{
			name:     "LeadingSlashOnInput",
			srcPath:  "/configs",
			destPath: "/out",
			input:    "/configs/foo",
			expected: "out/foo",
		},
		{
			name:     "SrcWithoutLeadingSlash",
			srcPath:  "configs",
			destPath: "out",
			input:    "configs/foo",
			expected: "out/foo",
		},
		{
			name:     "SrcPrefixNotDir",
			srcPath:  "/configs",
			destPath: "/",
			input:    "configs-extra/foo",
			expected: "configs-extra/foo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter := RewritePath(tt.srcPath, tt.destPath)
			h := &tar.Header{Name: tt.input}
			keep, err := filter(h)
			require.NoError(t, err)
			assert.True(t, keep)
			assert.Equal(t, tt.expected, h.Name)
		})
	}
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
