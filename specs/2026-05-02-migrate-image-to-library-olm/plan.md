# Implementation Plan

1. **Add library-olm dependency**
   - `go get github.com/joelanford/library-olm@main`
   - Verify dependency resolves cleanly with existing deps

2. **Create local ProgressRepository**
   - Add `internal/progress/progress.go` with a `ProgressRepository` that wraps `image.Repository` (the inner `ContainersImageRepository`) and intercepts `FetchBlob` to report bytes read via a `func(int)` callback
   - The wrapper calls the inner `FetchBlob`, wraps the returned `io.ReadCloser` to count bytes, and invokes the callback as data is consumed
   - Must wrap the inner client *before* `NewCachingRepository` — `CachingRepository` calls `inner.FetchBlob` on cache miss and copies the reader into the cache, so the callback fires at network speed during that copy
   - No `TotalLayerSize` helper needed — `Handler.Discover` provides the descriptor set, and summing `desc.Size` gives the total

3. **Update `internal/source/regv1.go`**
   - Replace `image.NewResolver()` + `Register(...)` + `resolver.Unpack(...)` with direct handler usage: `repo.Resolve` → `repo.FetchManifest` → `handler.Matches` → `handler.Unpack`
   - Use `imagebundle.RegistryV1Handler` from library-olm
   - Update `NewContainersImageClient` → `NewContainersImageRepository`
   - Update imports: `image` → `github.com/joelanford/library-olm/image`, add `imagebundle "github.com/joelanford/library-olm/image/bundle"`

4. **Update `internal/cmd/catalog.go`**
   - Replace `image.NewResolver()` + `Register(...)` + `resolver.TotalSize(...)` + `resolver.Unpack(...)` with Discover-based flow:
     1. Create `ContainersImageRepository`, wrap with `ProgressRepository`, wrap with `CachingRepository`
     2. `repo.Resolve` → `repo.FetchManifest` → `handler.Matches`
     3. `handler.Discover(ctx, repo, desc, manifestBytes)` → descriptor set
     4. `total := sumSizes(discoveredDescs)`
     5. `alreadyFetched := sumSizes(repo.CachedDescriptors())`
     6. Set up progress bar with `total` and `alreadyFetched` offset, connect to `ProgressRepository` callback
     7. `handler.Unpack(ctx, repo, desc, manifestBytes, dest)`
   - Use `imagecatalog.FBCHandler` from library-olm
   - Update `NewContainersImageClient` → `NewContainersImageRepository`
   - Replace `repo.SetOnBytesRead(...)` with `ProgressRepository` wrapper on inner client (before `CachingRepository`)
   - Update imports: add `imagecatalog "github.com/joelanford/library-olm/image/catalog"`
   - Apply same changes to both `catalogAddCmd` and `catalogUpdateCmd` callsites

5. **Delete `internal/image/`**
   - Remove the entire directory
   - Verify no remaining imports of `github.com/joelanford/orb/internal/image`

6. **Clean up**
   - `go mod tidy`
   - `make lint` — fix any issues
   - `make test` — fix any test failures
   - `make verify` — ensure no uncommitted changes
