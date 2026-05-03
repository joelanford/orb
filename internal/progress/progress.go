package progress

import (
	"context"
	"io"

	ocispecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"go.podman.io/image/v5/docker/reference"

	"github.com/joelanford/library-olm/image"
)

var _ image.Repository = (*Repository)(nil)

// Repository wraps an image.Repository and intercepts FetchBlob to report
// bytes read via a callback. It must wrap the inner client before it is
// passed to image.NewCachingRepository so the callback fires at network
// speed during cache-miss fetches.
type Repository struct {
	inner      image.Repository
	onReadFunc func(int)
}

// NewRepository wraps inner with byte-counting on blob reads.
func NewRepository(inner image.Repository) *Repository {
	return &Repository{inner: inner}
}

// SetOnRead sets the callback invoked with the number of bytes consumed
// on each Read from a blob returned by FetchBlob.
func (r *Repository) SetOnRead(fn func(int)) {
	r.onReadFunc = fn
}

func (r *Repository) Named() reference.Named         { return r.inner.Named() }
func (r *Repository) Close() error                    { return r.inner.Close() }
func (r *Repository) Resolve(ctx context.Context) (ocispecv1.Descriptor, error) {
	return r.inner.Resolve(ctx)
}
func (r *Repository) FetchManifest(ctx context.Context, desc ocispecv1.Descriptor) ([]byte, string, error) {
	return r.inner.FetchManifest(ctx, desc)
}

func (r *Repository) FetchBlob(ctx context.Context, desc ocispecv1.Descriptor) (io.ReadCloser, error) {
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
