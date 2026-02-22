package image

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/google/renameio/v2"
	ocispecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"go.podman.io/image/v5/docker/reference"
	"go.podman.io/image/v5/manifest"
)

// Repository is an image repository client that provides OCI registry access.
type Repository interface {
	// Named returns the reference of the repository. It must implement reference.NamedTagged or reference.Canonical.
	Named() reference.Named

	// Resolve gets the canonical digest for a reference.
	Resolve(ctx context.Context) (ocispecv1.Descriptor, error)

	// FetchManifest fetches raw manifest bytes and media type.
	FetchManifest(ctx context.Context, desc ocispecv1.Descriptor) ([]byte, string, error)

	// FetchBlob fetches a blob by digest.
	FetchBlob(ctx context.Context, desc ocispecv1.Descriptor) (io.ReadCloser, error)

	// Close releases any resources held by the repository.
	Close() error
}

// CachingRepository wraps a Repository with local caching of manifests and blobs.
type CachingRepository struct {
	inner Repository

	// Local storage
	cacheDir string

	// Parsed content cache
	resolution *ocispecv1.Descriptor

	// Optional callback invoked on each Read with the number of bytes read.
	onBytesRead func(int)
}

// SetOnBytesRead sets a callback that is invoked with the number of bytes
// read on each Read from a blob returned by FetchBlob.
func (s *CachingRepository) SetOnBytesRead(fn func(int)) {
	s.onBytesRead = fn
}

// NewCachingRepository creates a CachingRepository that caches fetched content locally.
func NewCachingRepository(client Repository) (*CachingRepository, error) {
	cacheDir, err := os.MkdirTemp("", "oci-session-")
	if err != nil {
		return nil, err
	}

	return &CachingRepository{
		inner:    client,
		cacheDir: cacheDir,
	}, nil
}

func (s *CachingRepository) Named() reference.Named {
	return s.inner.Named()
}

func (s *CachingRepository) Close() error {
	return errors.Join(s.inner.Close(), os.RemoveAll(s.cacheDir))
}

func (s *CachingRepository) manifestsDir() string {
	return filepath.Join(s.cacheDir, "manifests")
}

func (s *CachingRepository) blobsDir() string {
	return filepath.Join(s.cacheDir, "blobs")
}

func (s *CachingRepository) Resolve(ctx context.Context) (ocispecv1.Descriptor, error) {
	if s.resolution != nil {
		return *s.resolution, nil
	}

	desc, err := s.inner.Resolve(ctx)
	if err != nil {
		return ocispecv1.Descriptor{}, err
	}
	s.resolution = &desc
	return desc, nil
}

func (s *CachingRepository) FetchManifest(ctx context.Context, desc ocispecv1.Descriptor) ([]byte, string, error) {
	manifestsDir := s.manifestsDir()
	manifestPath := filepath.Join(manifestsDir, desc.Digest.String())
	manifestBytes, err := os.ReadFile(manifestPath)
	if err == nil {
		return manifestBytes, manifest.GuessMIMEType(manifestBytes), nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, "", err
	}

	// Fetch raw manifest
	raw, mediaType, err := s.inner.FetchManifest(ctx, desc)
	if err != nil {
		return nil, "", err
	}
	desc.MediaType = mediaType

	f, err := s.cacheFile(manifestPath, bytes.NewReader(raw))
	if err != nil {
		return nil, "", err
	}
	return raw, mediaType, f.Close()
}

func (s *CachingRepository) FetchBlob(ctx context.Context, desc ocispecv1.Descriptor) (io.ReadCloser, error) {
	blobDir := s.blobsDir()
	blobPath := filepath.Join(blobDir, desc.Digest.String())

	// Cache hit — wrap with callback so progress is reported (at disk speed)
	if f, err := os.Open(blobPath); err == nil {
		return s.wrapReader(f), nil
	}

	// Cache miss — wrap inner reader for progress during io.Copy (at network speed)
	reader, err := s.inner.FetchBlob(ctx, desc)
	if err != nil {
		return nil, err
	}

	f, err := s.cacheFile(blobPath, s.wrapReader(reader))
	return f, errors.Join(err, reader.Close())
}

func (s *CachingRepository) wrapReader(rc io.ReadCloser) io.ReadCloser {
	if s.onBytesRead == nil {
		return rc
	}
	return &callbackReadCloser{rc: rc, onRead: s.onBytesRead}
}

func (s *CachingRepository) cacheFile(path string, reader io.Reader) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return nil, err
	}
	f, err := renameio.TempFile("", path)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(f, reader); err != nil {
		return nil, errors.Join(err, f.Cleanup())
	}
	if err := f.CloseAtomicallyReplace(); err != nil {
		return nil, err
	}
	return os.Open(path)
}

// callbackReadCloser wraps an io.ReadCloser and invokes onRead with the
// number of bytes read on each Read call.
type callbackReadCloser struct {
	rc     io.ReadCloser
	onRead func(int)
}

func (cr *callbackReadCloser) Read(p []byte) (int, error) {
	n, err := cr.rc.Read(p)
	if n > 0 && cr.onRead != nil {
		cr.onRead(n)
	}
	return n, err
}

func (cr *callbackReadCloser) Close() error {
	return cr.rc.Close()
}

// ParseContent parses raw manifest bytes into a Content struct.
func ParseContent(desc ocispecv1.Descriptor, raw []byte) (*Content, error) {
	content := &Content{
		Descriptor: desc,
	}

	switch {
	case IsIndex(desc.MediaType):
		var idx ocispecv1.Index
		if err := json.Unmarshal(raw, &idx); err != nil {
			return nil, err
		}
		content.Index = &idx

	case IsManifest(desc.MediaType):
		var man ocispecv1.Manifest
		if err := json.Unmarshal(raw, &man); err != nil {
			return nil, err
		}
		content.Manifest = &man

	default:
		return nil, fmt.Errorf("unknown media type: %s", desc.MediaType)
	}

	return content, nil
}

// Content holds parsed OCI content (either an index or a manifest).
type Content struct {
	ocispecv1.Descriptor

	Index    *ocispecv1.Index
	Manifest *ocispecv1.Manifest
}

// IsIndex returns true if the media type represents an OCI index or Docker manifest list.
func IsIndex(mediaType string) bool {
	return mediaType == ocispecv1.MediaTypeImageIndex || mediaType == manifest.DockerV2ListMediaType
}

// IsManifest returns true if the media type represents an OCI manifest or Docker schema2 manifest.
func IsManifest(mediaType string) bool {
	return mediaType == ocispecv1.MediaTypeImageManifest || mediaType == manifest.DockerV2Schema2MediaType
}

// Handler knows how to unpack a specific type of OCI content.
type Handler interface {
	// Name returns a human-readable name for logging and debugging.
	Name() string

	// Matches returns true if this handler can handle the content.
	Matches(ctx context.Context, repo Repository, desc ocispecv1.Descriptor, manifestBytes []byte) bool

	// TotalSize returns the total compressed size of the blobs that will be fetched during Unpack.
	TotalSize(ctx context.Context, repo Repository, desc ocispecv1.Descriptor, manifestBytes []byte) (int64, error)

	// Unpack processes the content and writes the unpacked content to dest.
	Unpack(ctx context.Context, repo Repository, desc ocispecv1.Descriptor, manifestBytes []byte, dest string) error
}

// Resolver holds handlers and unpacks OCI content.
type Resolver struct {
	handlers []Handler
}

// NewResolver creates a new resolver.
func NewResolver() *Resolver {
	return &Resolver{}
}

// Register adds a handler to the resolver. Handlers are tried in registration
// order, so register higher-priority handlers first.
func (r *Resolver) Register(h Handler) {
	r.handlers = append(r.handlers, h)
}

// TotalSize resolves the content and returns the total compressed size
// of the blobs that will be fetched during Unpack, as determined by the
// first matching handler.
func (r *Resolver) TotalSize(ctx context.Context, repo Repository) (int64, error) {
	desc, manifestBytes, err := r.resolve(ctx, repo)
	if err != nil {
		return 0, err
	}

	for _, handler := range r.handlers {
		if handler.Matches(ctx, repo, desc, manifestBytes) {
			total, err := handler.TotalSize(ctx, repo, desc, manifestBytes)
			if err != nil {
				return 0, fmt.Errorf("handler %s: %w", handler.Name(), err)
			}
			return total, nil
		}
	}

	return 0, fmt.Errorf("no handler matched content (mediaType=%s, digest=%s)", desc.MediaType, desc.Digest)
}

// Unpack finds the first matching handler and unpacks content to the destination.
// The caller is responsible for creating and closing the session.
func (r *Resolver) Unpack(ctx context.Context, repo Repository, dest string) error {
	desc, manifestBytes, err := r.resolve(ctx, repo)
	if err != nil {
		return err
	}

	for _, handler := range r.handlers {
		if handler.Matches(ctx, repo, desc, manifestBytes) {
			if err := handler.Unpack(ctx, repo, desc, manifestBytes, dest); err != nil {
				return fmt.Errorf("handler %s: %w", handler.Name(), err)
			}
			return nil
		}
	}

	return fmt.Errorf("no handler matched content (mediaType=%s, digest=%s)", desc.MediaType, desc.Digest)
}

func (r *Resolver) resolve(ctx context.Context, repo Repository) (ocispecv1.Descriptor, []byte, error) {
	desc, err := repo.Resolve(ctx)
	if err != nil {
		return ocispecv1.Descriptor{}, nil, err
	}

	manifestBytes, mediaType, err := repo.FetchManifest(ctx, desc)
	if err != nil {
		return ocispecv1.Descriptor{}, nil, err
	}
	desc.MediaType = mediaType

	return desc, manifestBytes, nil
}
