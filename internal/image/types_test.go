package image

import (
	"encoding/json"
	"testing"

	ocispecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/image/v5/manifest"
)

func TestIsIndex(t *testing.T) {
	tests := []struct {
		name      string
		mediaType string
		want      bool
	}{
		{"OCIIndex", ocispecv1.MediaTypeImageIndex, true},
		{"DockerManifestList", manifest.DockerV2ListMediaType, true},
		{"OCIManifest", ocispecv1.MediaTypeImageManifest, false},
		{"Empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsIndex(tt.mediaType))
		})
	}
}

func TestIsManifest(t *testing.T) {
	tests := []struct {
		name      string
		mediaType string
		want      bool
	}{
		{"OCIManifest", ocispecv1.MediaTypeImageManifest, true},
		{"DockerSchema2", manifest.DockerV2Schema2MediaType, true},
		{"OCIIndex", ocispecv1.MediaTypeImageIndex, false},
		{"Empty", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsManifest(tt.mediaType))
		})
	}
}

func TestParseContent(t *testing.T) {
	t.Run("ValidIndex", func(t *testing.T) {
		idx := ocispecv1.Index{
			MediaType: ocispecv1.MediaTypeImageIndex,
			Manifests: []ocispecv1.Descriptor{
				{MediaType: ocispecv1.MediaTypeImageManifest},
			},
		}
		raw, err := json.Marshal(idx)
		require.NoError(t, err)

		desc := ocispecv1.Descriptor{MediaType: ocispecv1.MediaTypeImageIndex}
		content, err := ParseContent(desc, raw)
		require.NoError(t, err)
		assert.NotNil(t, content.Index)
		assert.Nil(t, content.Manifest)
	})

	t.Run("ValidManifest", func(t *testing.T) {
		man := ocispecv1.Manifest{
			MediaType: ocispecv1.MediaTypeImageManifest,
			Config:    ocispecv1.Descriptor{MediaType: ocispecv1.MediaTypeImageConfig},
		}
		raw, err := json.Marshal(man)
		require.NoError(t, err)

		desc := ocispecv1.Descriptor{MediaType: ocispecv1.MediaTypeImageManifest}
		content, err := ParseContent(desc, raw)
		require.NoError(t, err)
		assert.Nil(t, content.Index)
		assert.NotNil(t, content.Manifest)
	})

	t.Run("UnknownMediaType", func(t *testing.T) {
		desc := ocispecv1.Descriptor{MediaType: "application/unknown"}
		_, err := ParseContent(desc, []byte(`{}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown media type")
	})

	t.Run("InvalidJSON", func(t *testing.T) {
		desc := ocispecv1.Descriptor{MediaType: ocispecv1.MediaTypeImageIndex}
		_, err := ParseContent(desc, []byte(`{invalid`))
		require.Error(t, err)
	})
}

func TestNewResolver(t *testing.T) {
	r := NewResolver()
	assert.NotNil(t, r)
}

func TestResolver_Register(t *testing.T) {
	r := NewResolver()
	assert.Len(t, r.handlers, 0)

	r.Register(&RegistryV1Handler{})
	assert.Len(t, r.handlers, 1)

	r.Register(&RegistryV1Handler{})
	assert.Len(t, r.handlers, 2)
}
