# Requirements

- All plain manifest rendering uses `registryv1.ToPlainManifests()` from library-olm
- orb's `bundle.RegistryV1` type is replaced with `registryv1.Bundle` throughout the codebase
- `bundle.FromFS()` is replaced with `registryv1.FromFS()`
- Cert provider implementations come from library-olm (requires library-olm to export them first)
- `internal/convert/` is pruned to only retain types/functions needed by `internal/helm/`
- `internal/bundle/` retains only `registry.go`, `versionrelease.go`, and their tests
- No behavioral changes — rendered manifests are identical before and after

## Acceptance Criteria

- `make test` passes
- `make lint` passes
- `make verify` passes
- `orb bundle convert plain` produces identical output to before the migration (spot-check with a real bundle)
- No imports of `internal/convert` remain outside `internal/convert/` and `internal/helm/`
- No imports of `internal/bundle` for the `RegistryV1` type (only for `VersionRelease`, `Property`, etc.)
- `internal/convert/certproviders/` is deleted
- `internal/bundle/bundle.go` and `internal/bundle/fromfs.go` are deleted
