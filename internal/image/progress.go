package image

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"time"

	ocispecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/vbauerster/mpb/v8"
	"go.podman.io/image/v5/docker/reference"
	"go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/types"
)

// ProgressRepository wraps a Repository, intercepting FetchBlob to report
// download progress via an mpb.Bar. All other methods pass through unchanged.
type ProgressRepository struct {
	inner Repository
	bar   *mpb.Bar
}

// NewProgressRepository wraps inner so that every FetchBlob read increments bar.
func NewProgressRepository(inner Repository, bar *mpb.Bar) *ProgressRepository {
	return &ProgressRepository{inner: inner, bar: bar}
}

func (p *ProgressRepository) Named() reference.Named { return p.inner.Named() }
func (p *ProgressRepository) Close() error           { return p.inner.Close() }
func (p *ProgressRepository) Resolve(ctx context.Context) (ocispecv1.Descriptor, error) {
	return p.inner.Resolve(ctx)
}
func (p *ProgressRepository) FetchManifest(ctx context.Context, desc ocispecv1.Descriptor) ([]byte, string, error) {
	return p.inner.FetchManifest(ctx, desc)
}

func (p *ProgressRepository) FetchBlob(ctx context.Context, desc ocispecv1.Descriptor) (io.ReadCloser, error) {
	rc, err := p.inner.FetchBlob(ctx, desc)
	if err != nil {
		return nil, err
	}
	return &progressReadCloser{rc: rc, bar: p.bar, lastFlush: time.Now()}, nil
}

// progressReadCloser wraps an io.ReadCloser and increments an mpb.Bar on each
// Read. To avoid spiky speed readings, bytes are accumulated and flushed to the
// bar at most once per flushInterval.
type progressReadCloser struct {
	rc  io.ReadCloser
	bar *mpb.Bar

	pending   int
	lastFlush time.Time
}

const flushInterval = 500 * time.Millisecond

func (pr *progressReadCloser) Read(p []byte) (int, error) {
	n, err := pr.rc.Read(p)
	if n > 0 {
		now := time.Now()
		pr.pending += n
		if elapsed := now.Sub(pr.lastFlush); elapsed >= flushInterval || err != nil {
			pr.bar.EwmaIncrBy(pr.pending, elapsed)
			pr.pending = 0
			pr.lastFlush = now
		}
	}
	return n, err
}

func (pr *progressReadCloser) Close() error {
	if pr.pending > 0 {
		pr.bar.EwmaIncrBy(pr.pending, time.Since(pr.lastFlush))
		pr.pending = 0
	}
	return pr.rc.Close()
}

// TotalLayerSize computes the total compressed layer size for a resolved descriptor.
// If desc points to a manifest index, it resolves to the current platform's manifest first.
func TotalLayerSize(ctx context.Context, repo Repository, desc ocispecv1.Descriptor, manifestBytes []byte, mediaType string) (int64, error) {
	if IsIndex(mediaType) {
		list, err := manifest.ListFromBlob(manifestBytes, mediaType)
		if err != nil {
			return 0, fmt.Errorf("parsing manifest list: %w", err)
		}
		chosenDigest, err := list.ChooseInstance(&types.SystemContext{
			OSChoice:           "linux",
			ArchitectureChoice: runtime.GOARCH,
		})
		if err != nil {
			return 0, fmt.Errorf("choosing platform instance: %w", err)
		}
		instanceInfo, err := list.Instance(chosenDigest)
		if err != nil {
			return 0, fmt.Errorf("getting instance info: %w", err)
		}
		platformDesc := ocispecv1.Descriptor{
			MediaType: instanceInfo.MediaType,
			Digest:    chosenDigest,
			Size:      instanceInfo.Size,
		}
		manifestBytes, _, err = repo.FetchManifest(ctx, platformDesc)
		if err != nil {
			return 0, fmt.Errorf("fetching platform manifest: %w", err)
		}
	}

	var m ocispecv1.Manifest
	if err := json.Unmarshal(manifestBytes, &m); err != nil {
		return 0, fmt.Errorf("parsing manifest: %w", err)
	}

	var total int64
	for _, layer := range m.Layers {
		total += layer.Size
	}
	return total, nil
}
