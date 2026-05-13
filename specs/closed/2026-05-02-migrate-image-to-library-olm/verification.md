# Verification

## Implementation Correctness

- [ ] `internal/image/` directory is deleted
- [ ] `grep -r 'internal/image' --include='*.go'` returns no results
- [ ] `go.mod` lists `github.com/joelanford/library-olm` in require block
- [ ] `internal/progress/` exists with `ProgressRepository` wrapping inner `Repository` (not `CachingRepository`)
- [ ] `ProgressRepository` intercepts `FetchBlob` to report bytes read via callback at network speed
- [ ] `internal/source/regv1.go` uses `imagebundle.RegistryV1Handler` directly (no Discover needed)
- [ ] `internal/cmd/catalog.go` uses `imagecatalog.FBCHandler` directly
- [ ] `internal/cmd/catalog.go` calls `handler.Discover` to get descriptor set for total size
- [ ] `internal/cmd/catalog.go` calls `repo.CachedDescriptors()` to determine already-fetched content
- [ ] `internal/cmd/catalog.go` wraps inner `ContainersImageRepository` with `ProgressRepository` before passing to `NewCachingRepository`
- [ ] No references to `ContainersImageClient` — all replaced with `ContainersImageRepository`
- [ ] No references to `ForceOwnershipRWX` — all replaced with `AsCurrentUser`
- [ ] No manual platform manifest resolution or `TotalLayerSize` helper in orb

## Project Conventions

- [ ] `make test` passes
- [ ] `make lint` passes
- [ ] `make verify` passes
- [ ] No exported Go APIs added (per specs/mission.md: "No exported Go APIs beyond the CLI binary itself")
- [ ] All logic in `internal/` (per specs/mission.md)
- [ ] Commit messages follow conventional commits format (per specs/conventions.md)
