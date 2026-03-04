package image

import (
	"testing"

	ocispecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
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
