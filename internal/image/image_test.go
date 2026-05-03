package image

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"

	libraryimage "github.com/joelanford/library-olm/image"
	"github.com/opencontainers/go-digest"
	ocispecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.podman.io/image/v5/docker/reference"
)

type fakeRepo struct {
	named        reference.Named
	resolveDesc  ocispecv1.Descriptor
	resolveErr   error
	manifests    map[string]fakeManifest
	blobs        map[string][]byte
	closeErr     error
	fetchBlobErr error
}

type fakeManifest struct {
	data      []byte
	mediaType string
}

func newFakeRepo() *fakeRepo {
	ref, _ := reference.ParseNormalizedNamed("example.com/test:latest")
	return &fakeRepo{
		named:     ref,
		manifests: make(map[string]fakeManifest),
		blobs:     make(map[string][]byte),
	}
}

func (r *fakeRepo) Named() reference.Named { return r.named }
func (r *fakeRepo) Close() error           { return r.closeErr }

func (r *fakeRepo) Resolve(_ context.Context) (ocispecv1.Descriptor, error) {
	return r.resolveDesc, r.resolveErr
}

func (r *fakeRepo) FetchManifest(_ context.Context, desc ocispecv1.Descriptor) ([]byte, string, error) {
	m, ok := r.manifests[desc.Digest.String()]
	if !ok {
		return nil, "", errors.New("manifest not found")
	}
	return m.data, m.mediaType, nil
}

func (r *fakeRepo) FetchBlob(_ context.Context, desc ocispecv1.Descriptor) (io.ReadCloser, error) {
	if r.fetchBlobErr != nil {
		return nil, r.fetchBlobErr
	}
	data, ok := r.blobs[desc.Digest.String()]
	if !ok {
		return nil, errors.New("blob not found")
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (r *fakeRepo) addManifest(data []byte, mediaType string) ocispecv1.Descriptor {
	d := digest.FromBytes(data)
	r.manifests[d.String()] = fakeManifest{data: data, mediaType: mediaType}
	return ocispecv1.Descriptor{Digest: d, Size: int64(len(data)), MediaType: mediaType}
}

func (r *fakeRepo) addBlob(data []byte) ocispecv1.Descriptor {
	d := digest.FromBytes(data)
	r.blobs[d.String()] = data
	return ocispecv1.Descriptor{Digest: d, Size: int64(len(data))}
}

type fakeHandler struct {
	name     string
	matches  bool
	matchErr error
}

func (h *fakeHandler) Name() string { return h.name }

func (h *fakeHandler) Matches(_ context.Context, _ libraryimage.Repository, _ ocispecv1.Descriptor, _ []byte) (bool, error) {
	return h.matches, h.matchErr
}

func (h *fakeHandler) Discover(_ context.Context, _ libraryimage.Repository, _ ocispecv1.Descriptor, _ []byte) ([]ocispecv1.Descriptor, error) {
	return nil, nil
}

func (h *fakeHandler) Unpack(_ context.Context, _ libraryimage.Repository, _ ocispecv1.Descriptor, _ []byte, _ string) error {
	return nil
}

func TestNewRepository(t *testing.T) {
	inner := newFakeRepo()
	repo, err := NewRepository(inner)
	require.NoError(t, err)
	require.NotNil(t, repo)
	assert.Equal(t, inner.named, repo.Named())
	require.NoError(t, repo.Close())
}

func TestRepository_SetOnRead_FetchBlob(t *testing.T) {
	inner := newFakeRepo()
	blobData := []byte("hello world")
	desc := inner.addBlob(blobData)

	repo, err := NewRepository(inner)
	require.NoError(t, err)
	defer repo.Close()

	var totalRead int
	repo.SetOnRead(func(n int) { totalRead += n })

	rc, err := repo.FetchBlob(context.Background(), desc)
	require.NoError(t, err)
	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())

	assert.Equal(t, blobData, got)
	assert.Equal(t, len(blobData), totalRead)
}

func TestRepository_SetOnRead_FetchManifest(t *testing.T) {
	inner := newFakeRepo()
	manifestData := []byte(`{"schemaVersion":2}`)
	desc := inner.addManifest(manifestData, ocispecv1.MediaTypeImageManifest)

	repo, err := NewRepository(inner)
	require.NoError(t, err)
	defer repo.Close()

	var totalRead int
	repo.SetOnRead(func(n int) { totalRead += n })

	got, mediaType, err := repo.FetchManifest(context.Background(), desc)
	require.NoError(t, err)
	assert.Equal(t, manifestData, got)
	assert.Equal(t, ocispecv1.MediaTypeImageManifest, mediaType)
	assert.Equal(t, len(manifestData), totalRead)
}

func TestRepository_NoOnRead_Passthrough(t *testing.T) {
	inner := newFakeRepo()
	blobData := []byte("no callback set")
	desc := inner.addBlob(blobData)

	repo, err := NewRepository(inner)
	require.NoError(t, err)
	defer repo.Close()

	rc, err := repo.FetchBlob(context.Background(), desc)
	require.NoError(t, err)
	got, err := io.ReadAll(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())

	assert.Equal(t, blobData, got)
}

func TestRepository_FetchBlob_MultipleReads(t *testing.T) {
	inner := newFakeRepo()
	blobData := bytes.Repeat([]byte("x"), 1024)
	desc := inner.addBlob(blobData)

	repo, err := NewRepository(inner)
	require.NoError(t, err)
	defer repo.Close()

	var totalRead int
	repo.SetOnRead(func(n int) { totalRead += n })

	rc, err := repo.FetchBlob(context.Background(), desc)
	require.NoError(t, err)

	buf := make([]byte, 100)
	var collected []byte
	for {
		n, readErr := rc.Read(buf)
		if n > 0 {
			collected = append(collected, buf[:n]...)
		}
		if readErr != nil {
			break
		}
	}
	require.NoError(t, rc.Close())

	assert.Equal(t, blobData, collected)
	assert.Equal(t, len(blobData), totalRead)
}

func TestRepository_Resolve(t *testing.T) {
	inner := newFakeRepo()
	expectedDesc := ocispecv1.Descriptor{Digest: digest.FromString("test")}
	inner.resolveDesc = expectedDesc

	repo, err := NewRepository(inner)
	require.NoError(t, err)
	defer repo.Close()

	got, err := repo.Resolve(context.Background())
	require.NoError(t, err)
	assert.Equal(t, expectedDesc.Digest, got.Digest)
}

func TestRepository_CachedDescriptors(t *testing.T) {
	inner := newFakeRepo()
	blobData := []byte("cached content")
	desc := inner.addBlob(blobData)

	repo, err := NewRepository(inner)
	require.NoError(t, err)
	defer repo.Close()

	// Before any fetch, cache is empty.
	assert.Empty(t, repo.CachedDescriptors())

	// Fetch a blob to populate the cache.
	rc, err := repo.FetchBlob(context.Background(), desc)
	require.NoError(t, err)
	_, err = io.ReadAll(rc)
	require.NoError(t, err)
	require.NoError(t, rc.Close())

	cached := repo.CachedDescriptors()
	require.Len(t, cached, 1)
	assert.Equal(t, desc.Digest, cached[0].Digest)
}

func TestResolveAndMatch_Success(t *testing.T) {
	inner := newFakeRepo()
	manifestData := []byte(`{"schemaVersion":2}`)
	desc := inner.addManifest(manifestData, ocispecv1.MediaTypeImageManifest)
	inner.resolveDesc = desc

	handler := &fakeHandler{name: "test", matches: true}
	gotDesc, gotBytes, err := ResolveAndMatch(context.Background(), inner, handler)
	require.NoError(t, err)
	assert.Equal(t, manifestData, gotBytes)
	assert.Equal(t, desc.Digest, gotDesc.Digest)
	assert.Equal(t, ocispecv1.MediaTypeImageManifest, gotDesc.MediaType)
}

func TestResolveAndMatch_ResolveError(t *testing.T) {
	inner := newFakeRepo()
	inner.resolveErr = errors.New("network down")

	handler := &fakeHandler{name: "test", matches: true}
	_, _, err := ResolveAndMatch(context.Background(), inner, handler)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolving image")
}

func TestResolveAndMatch_FetchManifestError(t *testing.T) {
	inner := newFakeRepo()
	inner.resolveDesc = ocispecv1.Descriptor{
		Digest: digest.FromString("missing"),
	}

	handler := &fakeHandler{name: "test", matches: true}
	_, _, err := ResolveAndMatch(context.Background(), inner, handler)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "fetching manifest")
}

func TestResolveAndMatch_NoMatch(t *testing.T) {
	inner := newFakeRepo()
	manifestData := []byte(`{"schemaVersion":2}`)
	desc := inner.addManifest(manifestData, ocispecv1.MediaTypeImageManifest)
	inner.resolveDesc = desc

	handler := &fakeHandler{name: "fbc", matches: false}
	_, _, err := ResolveAndMatch(context.Background(), inner, handler)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match fbc handler")
}

func TestResolveAndMatch_MatchError(t *testing.T) {
	inner := newFakeRepo()
	manifestData := []byte(`{"schemaVersion":2}`)
	desc := inner.addManifest(manifestData, ocispecv1.MediaTypeImageManifest)
	inner.resolveDesc = desc

	handler := &fakeHandler{name: "test", matchErr: errors.New("match failed")}
	_, _, err := ResolveAndMatch(context.Background(), inner, handler)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "checking image type")
}
