package image

import (
	"context"
	"io"

	libraryimage "github.com/joelanford/library-olm/image"
	ocispecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"go.podman.io/image/v5/docker/reference"
)

var _ libraryimage.Repository = (*progressRepository)(nil)

type progressRepository struct {
	inner      libraryimage.Repository
	onReadFunc func(int)
}

func newProgressRepository(inner libraryimage.Repository) *progressRepository {
	return &progressRepository{inner: inner}
}

func (r *progressRepository) setOnRead(fn func(int)) {
	r.onReadFunc = fn
}

func (r *progressRepository) Named() reference.Named { return r.inner.Named() }
func (r *progressRepository) Close() error           { return r.inner.Close() }
func (r *progressRepository) Resolve(ctx context.Context) (ocispecv1.Descriptor, error) {
	return r.inner.Resolve(ctx)
}
func (r *progressRepository) FetchManifest(ctx context.Context, desc ocispecv1.Descriptor) ([]byte, string, error) {
	data, mediaType, err := r.inner.FetchManifest(ctx, desc)
	if err == nil && r.onReadFunc != nil {
		r.onReadFunc(len(data))
	}
	return data, mediaType, err
}

func (r *progressRepository) FetchBlob(ctx context.Context, desc ocispecv1.Descriptor) (io.ReadCloser, error) {
	rc, err := r.inner.FetchBlob(ctx, desc)
	if err != nil || r.onReadFunc == nil {
		return rc, err
	}
	return &countingReadCloser{rc: rc, onRead: r.onReadFunc}, nil
}

type countingReadCloser struct {
	rc     io.ReadCloser
	onRead func(int)
}

func (c *countingReadCloser) Read(p []byte) (int, error) {
	n, err := c.rc.Read(p)
	if n > 0 {
		c.onRead(n)
	}
	return n, err
}

func (c *countingReadCloser) Close() error {
	return c.rc.Close()
}
