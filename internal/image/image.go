package image

import (
	"context"
	"fmt"
	"io"

	libraryimage "github.com/joelanford/library-olm/image"
	ocispecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"go.podman.io/image/v5/docker/reference"
)

var _ libraryimage.Repository = (*Repository)(nil)

// Repository combines progress-tracking and caching layers into a single
// repository. It implements libraryimage.Repository so it can be passed
// directly to handlers and ResolveAndMatch.
type Repository struct {
	progress *progressRepository
	caching  *libraryimage.CachingRepository
}

// NewRepository wraps base with progress-tracking and caching layers.
func NewRepository(base libraryimage.Repository) (*Repository, error) {
	pr := newProgressRepository(base)
	cr, err := libraryimage.NewCachingRepository(pr)
	if err != nil {
		return nil, fmt.Errorf("creating caching repository: %w", err)
	}
	return &Repository{progress: pr, caching: cr}, nil
}

// SetOnRead sets the callback invoked with the number of bytes consumed
// on each Read from a blob returned by FetchBlob.
func (r *Repository) SetOnRead(fn func(int)) { r.progress.setOnRead(fn) }

// CachedDescriptors returns descriptors for all content currently in the cache.
func (r *Repository) CachedDescriptors() []ocispecv1.Descriptor {
	return r.caching.CachedDescriptors()
}

func (r *Repository) Named() reference.Named { return r.caching.Named() }
func (r *Repository) Close() error           { return r.caching.Close() }
func (r *Repository) Resolve(ctx context.Context) (ocispecv1.Descriptor, error) {
	return r.caching.Resolve(ctx)
}
func (r *Repository) FetchManifest(ctx context.Context, desc ocispecv1.Descriptor) ([]byte, string, error) {
	return r.caching.FetchManifest(ctx, desc)
}
func (r *Repository) FetchBlob(ctx context.Context, desc ocispecv1.Descriptor) (io.ReadCloser, error) {
	return r.caching.FetchBlob(ctx, desc)
}

// ResolveAndMatch resolves the image reference in repo, fetches the manifest,
// and checks that it matches the given handler. It returns the resolved
// descriptor and raw manifest bytes on success.
func ResolveAndMatch(ctx context.Context, repo libraryimage.Repository, handler libraryimage.Handler) (ocispecv1.Descriptor, []byte, error) {
	desc, err := repo.Resolve(ctx)
	if err != nil {
		return ocispecv1.Descriptor{}, nil, fmt.Errorf("resolving image: %w", err)
	}

	manifestBytes, mediaType, err := repo.FetchManifest(ctx, desc)
	if err != nil {
		return ocispecv1.Descriptor{}, nil, fmt.Errorf("fetching manifest: %w", err)
	}
	desc.MediaType = mediaType

	if matched, err := handler.Matches(ctx, repo, desc, manifestBytes); err != nil {
		return ocispecv1.Descriptor{}, nil, fmt.Errorf("checking image type: %w", err)
	} else if !matched {
		return ocispecv1.Descriptor{}, nil, fmt.Errorf("image does not match %s handler", handler.Name())
	}

	return desc, manifestBytes, nil
}
