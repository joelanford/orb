---
status: in-progress
---
# Migrate image layer to library-olm

## Summary

Replace orb's `internal/image/` package with `github.com/joelanford/library-olm/image` and its sub-packages (`image/bundle`, `image/catalog`). This eliminates ~500 lines of duplicated OCI image handling code while gaining library-olm's improvements (singleflight-based dedup, thread-safe in-memory manifest caching, error-returning `Matches`, `Discover` for upfront size knowledge, and `CachedDescriptors` for cache introspection).

No changes to library-olm are required. Progress tracking uses library-olm's `Handler.Discover` and `CachingRepository.CachedDescriptors` to determine total size and already-fetched content, with a local `ProgressRepository` decorator for byte-level reporting during `Unpack`.

## Design

### Image types and handlers

All usages of orb's `internal/image` types switch to library-olm imports:

| orb (remove) | library-olm (use instead) |
|---|---|
| `image.Repository` | `image.Repository` (identical interface) |
| `image.CachingRepository` | `image.CachingRepository` (better: singleflight, atomic, in-memory manifests, `CachedDescriptors`) |
| `image.NewCachingRepository` | `image.NewCachingRepository` (same signature) |
| `image.ContainersImageClient` | `image.ContainersImageRepository` (renamed) |
| `image.NewContainersImageClient` | `image.NewContainersImageRepository` (renamed, takes `ctx` + `ImageReference` + `SystemContext`) |
| `image.FetchImageConfig` | `image.FetchImageConfig` (same) |
| `image.ManifestUnpacker` | Removed in library-olm — logic moved to internal `ociutil.ApplyLayers` |
| `image.IsIndex` / `image.IsManifest` | `image.IsIndex` / `image.IsManifest` (same) |
| `image.CombineFilters` | `image.CombineFilters` (same) |
| `image.OnlyPaths` | `image.OnlyPaths` (same) |
| `image.RewritePath` | `image.RewritePath` (same) |
| `image.ForceOwnershipRWX` | `image.AsCurrentUser` (compatible behavior) |
| `image.RegistryV1Handler` | `imagebundle.RegistryV1Handler` (`image/bundle` sub-package) |
| `image.FBCHandler` | `imagecatalog.FBCHandler` (`image/catalog` sub-package) |

### Direct Handler usage

orb's `Resolver` (with `NewResolver()` + `Register()` + `TotalSize()`) is replaced by direct `Handler` usage. Each callsite knows which handler to use (`FBCHandler` for catalogs, `RegistryV1Handler` for bundles) and drives it directly: resolve, match, discover, unpack.

`Matches` is called as a safeguard — orb knows which handler to use, but if the image ref doesn't match expectations (e.g., a non-FBC image passed to `catalog add`), `Matches` lets us fail fast with a clear error rather than falling through to a confusing `Discover` or `Unpack` failure.

`Discover` is the key new capability: it walks the image tree and returns all descriptors needed to unpack, giving the caller upfront knowledge of the total download size. This is what enables meaningful progress bars.

Key API difference: library-olm's `Handler.Matches` returns `(bool, error)` instead of orb's `bool`. This is a strict improvement — errors during matching are reported rather than silently swallowed.

### Progress tracking via Discover + CachedDescriptors

Library-olm provides two building blocks that eliminate orb's `SetOnBytesRead` callback approach:

1. **`Handler.Discover(ctx, repo, desc, manifestBytes) ([]ocispecv1.Descriptor, error)`** — walks the image tree (indices, manifests, configs) and returns the complete set of descriptors needed to unpack. Fetches only manifests and configs (not layer blobs). When used with `CachingRepository`, all fetched content is cached and reused by `Unpack` for free.

2. **`CachingRepository.CachedDescriptors() []ocispecv1.Descriptor`** — returns a snapshot of all descriptors currently in the cache (manifests + blobs). The caller can intersect with the discovered set to determine what's already been fetched.

**Approach**: orb defines a local `ProgressRepository` that wraps the inner `ContainersImageRepository` *before* it's passed to `NewCachingRepository`. This works because `CachingRepository` calls `inner.FetchBlob` on cache miss and `io.Copy`s the returned reader into the cache file — the progress callback fires during that copy at network speed. Wrapping *after* `CachingRepository` would only see fast disk reads from the cache, missing the actual download.

```
ContainersImageRepository → ProgressRepository → CachingRepository → Handler.Unpack
```

The total download size is known upfront from `Discover`, and already-cached content (manifests and configs fetched during `Discover`) is accounted for via `CachedDescriptors`.

The flow at each callsite:

1. Create `ContainersImageRepository`, wrap with `ProgressRepository`, wrap with `CachingRepository`
2. `repo.Resolve(ctx)` → descriptor
3. `repo.FetchManifest(ctx, desc)` → manifest bytes + media type
4. `handler.Matches(ctx, repo, desc, manifestBytes)` → validates image type
5. `handler.Discover(ctx, repo, desc, manifestBytes)` → all descriptors (manifests/configs cached as side effect)
6. Compute total size: `sum(desc.Size for desc in discoveredDescs)`
7. Compute already-fetched size: `sum(desc.Size for desc in repo.CachedDescriptors())`
8. Set up progress bar with total and initial offset, connect to `ProgressRepository` callback
9. `handler.Unpack(ctx, repo, desc, manifestBytes, dest)`

This eliminates the need for orb to duplicate platform manifest resolution or layer size summation — `Discover` handles the tree walking. The progress bar shows two phases:

1. **Discovering** — spinner while `Discover` walks the tree
2. **Downloading** — progress bar from `alreadyFetched/total` to `total/total`

### Catalog path

The catalog add/update code uses the Discover-based flow above. No platform manifest resolution duplication needed — `Discover` returns the exact set of descriptors that `Unpack` will need, regardless of whether the image is an index or a plain manifest.

### Bundle source path

The bundle source code (`internal/source/regv1.go`) uses the handler directly without progress tracking: resolve, match, unpack. No `Discover` call needed since there's no progress bar.

### Callsites to update

There are three callsites that use `internal/image`:

1. **`internal/cmd/catalog.go` — `catalogAddCmd`** (line ~503): Creates `Resolver`, calls `TotalSize`, sets `OnBytesRead`, calls `Unpack`. Replace with `Discover`-based progress flow using `FBCHandler` and `ProgressRepository`.

2. **`internal/cmd/catalog.go` — `catalogUpdateCmd`** (line ~705): Same pattern as catalogAddCmd but inside a parallel worker loop. Same changes.

3. **`internal/source/regv1.go` — `readFromImage`** (line ~187): Creates `Resolver`, registers `RegistryV1Handler`, calls `Unpack`. Replace with direct `RegistryV1Handler` usage (resolve → match → unpack, no Discover needed).

### What gets deleted

The entire `internal/image/` directory (7 files):
- `types.go` — Repository, CachingRepository, Handler, Resolver, callbackReadCloser
- `containers_image_client.go` — ContainersImageClient
- `handler_regv1.go` — RegistryV1Handler
- `handler_fbc.go` — FBCHandler, resolvePlatformManifest
- `image_manifest.go` — FetchImageConfig, ManifestUnpacker, layer filters, totalLayerSize
- `types_test.go`
- `image_manifest_test.go`
