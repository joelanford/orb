package image

import (
	"context"
	"testing"

	ocispecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"go.podman.io/image/v5/manifest"
)

type stubHandler struct{}

func (s *stubHandler) Name() string { return "stub" }
func (s *stubHandler) Matches(_ context.Context, _ Repository, _ ocispecv1.Descriptor, _ []byte) bool {
	return false
}
func (s *stubHandler) TotalSize(_ context.Context, _ Repository, _ ocispecv1.Descriptor, _ []byte) (int64, error) {
	return 0, nil
}
func (s *stubHandler) Unpack(_ context.Context, _ Repository, _ ocispecv1.Descriptor, _ []byte, _ string) error {
	return nil
}

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

	r.Register(&stubHandler{})
	assert.Len(t, r.handlers, 1)

	r.Register(&stubHandler{})
	assert.Len(t, r.handlers, 2)
}
