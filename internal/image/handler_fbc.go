package image

import (
	"context"
	"fmt"
	"runtime"

	ocispecv1 "github.com/opencontainers/image-spec/specs-go/v1"
	"go.podman.io/image/v5/manifest"
	"go.podman.io/image/v5/types"
)

// ConfigDirLabel is the label on catalog images that specifies the directory
// containing the catalog configuration.
const ConfigDirLabel = "operators.operatorframework.io.index.configs.v1"

// FBCHandler unpacks FBC catalog images by extracting only the configs directory.
// It handles both single-platform manifests and multi-platform manifest lists/indexes.
type FBCHandler struct{}

func (h *FBCHandler) Name() string { return "olm.operatorframework.io/fbc+v0" }

func (h *FBCHandler) Matches(ctx context.Context, repo Repository, desc ocispecv1.Descriptor, manifestBytes []byte) bool {
	// If this is a manifest list/index, resolve to the platform-specific manifest first
	if IsIndex(desc.MediaType) {
		platformDesc, platformManifestBytes, err := resolvePlatformManifest(ctx, repo, manifestBytes, desc.MediaType)
		if err != nil {
			return false
		}
		desc = platformDesc
		manifestBytes = platformManifestBytes
	}

	if !IsManifest(desc.MediaType) {
		return false
	}

	cfg, err := FetchImageConfig(ctx, repo, manifestBytes)
	if err != nil {
		return false
	}

	_, ok := cfg.Config.Labels[ConfigDirLabel]
	return ok
}

func (h *FBCHandler) TotalSize(ctx context.Context, repo Repository, desc ocispecv1.Descriptor, manifestBytes []byte) (int64, error) {
	if IsIndex(desc.MediaType) {
		_, platformManifestBytes, err := resolvePlatformManifest(ctx, repo, manifestBytes, desc.MediaType)
		if err != nil {
			return 0, fmt.Errorf("resolving platform manifest: %w", err)
		}
		manifestBytes = platformManifestBytes
	}
	return totalLayerSize(manifestBytes)
}

func (h *FBCHandler) Unpack(ctx context.Context, repo Repository, desc ocispecv1.Descriptor, manifestBytes []byte, dest string) error {
	// If this is a manifest list/index, resolve to the platform-specific manifest first
	if IsIndex(desc.MediaType) {
		platformDesc, platformManifestBytes, err := resolvePlatformManifest(ctx, repo, manifestBytes, desc.MediaType)
		if err != nil {
			return fmt.Errorf("resolving platform manifest: %w", err)
		}
		desc = platformDesc
		manifestBytes = platformManifestBytes
	}

	cfg, err := FetchImageConfig(ctx, repo, manifestBytes)
	if err != nil {
		return err
	}

	configDir := cfg.Config.Labels[ConfigDirLabel]

	unpacker := &ManifestUnpacker{
		Filter: CombineFilters(
			OnlyPaths(configDir),
			ForceOwnershipRWX(),
		),
	}
	return unpacker.Unpack(ctx, repo, manifestBytes, dest)
}

// resolvePlatformManifest selects the appropriate platform manifest from a manifest list/index.
func resolvePlatformManifest(ctx context.Context, repo Repository, indexBytes []byte, indexMediaType string) (ocispecv1.Descriptor, []byte, error) {
	list, err := manifest.ListFromBlob(indexBytes, indexMediaType)
	if err != nil {
		return ocispecv1.Descriptor{}, nil, fmt.Errorf("parsing manifest list: %w", err)
	}

	// Use linux as the OS since catalog images are linux container images.
	// The architecture is inherited from the current runtime.
	chosenDigest, err := list.ChooseInstance(&types.SystemContext{
		OSChoice:           "linux",
		ArchitectureChoice: runtime.GOARCH,
	})
	if err != nil {
		return ocispecv1.Descriptor{}, nil, fmt.Errorf("choosing platform instance: %w", err)
	}

	instanceInfo, err := list.Instance(chosenDigest)
	if err != nil {
		return ocispecv1.Descriptor{}, nil, fmt.Errorf("getting instance info: %w", err)
	}

	desc := ocispecv1.Descriptor{
		MediaType: instanceInfo.MediaType,
		Digest:    chosenDigest,
		Size:      instanceInfo.Size,
	}

	manifestBytes, mediaType, err := repo.FetchManifest(ctx, desc)
	if err != nil {
		return ocispecv1.Descriptor{}, nil, fmt.Errorf("fetching platform manifest: %w", err)
	}
	desc.MediaType = mediaType

	return desc, manifestBytes, nil
}
