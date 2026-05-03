# Requirements

- All `internal/image/` types, functions, and handlers are replaced by `library-olm/image` equivalents
- `internal/image/` directory is deleted entirely — no shims, re-exports, or compatibility wrappers
- Download progress bars for `orb catalog add` and `orb catalog update` continue to work
- Progress uses a two-phase UX: spinner during `Discover`, progress bar during `Unpack`
- Total download size is determined upfront via `Handler.Discover` — no manual platform manifest resolution or layer size summation in orb
- Already-cached content (manifests/configs from `Discover`) is accounted for via `CachingRepository.CachedDescriptors`
- Byte-level progress is reported via a local `ProgressRepository` wrapping the inner `ContainersImageRepository` before `CachingRepository` — so the callback fires at network speed during cache-miss fetches
- All existing transports (docker://, oci:, oci-archive:, dir:, tar:) continue to work for bundle sources
- `ContainersImageClient` usages are replaced with `ContainersImageRepository`
- `ForceOwnershipRWX` usages are replaced with `AsCurrentUser`

## Acceptance Criteria

- `make test` passes with no test failures
- `make lint` passes with no lint errors
- `make verify` passes (no uncommitted tidy/format changes)
- `internal/image/` directory does not exist
- No remaining imports of `github.com/joelanford/orb/internal/image` anywhere in the codebase
- `go.mod` includes `github.com/joelanford/library-olm` as a dependency
- `orb catalog add operatorhubio docker://quay.io/operatorhubio/catalog:latest` shows a two-phase progress (discovering spinner + download progress bar) and completes successfully
- `orb catalog update` shows progress for each catalog and completes successfully
- `orb bundle convert plain docker://<some-bundle-image> -n operators` produces correct output
- `orb catalog resolve <package>` returns correct results after catalog add
