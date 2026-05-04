---
status: idea
---
# Migrate plain manifest conversion to library-olm

Replace orb's `internal/convert/` package with library-olm's `bundle/registry/v1.ToPlainManifests()` for plain manifest rendering. library-olm has near-identical conversion logic (validators, resource generators, cert providers) already implemented internally.

Callers to update:
- `internal/cmd/bundle.go` — uses `convert.Converter.Convert()`, `convert.Option`, cert provider implementations
- `internal/destination/plain.go` — uses `convert.Converter.Convert()`
- `internal/destination/destination.go` — uses `convert.Option` type

The helm package also depends on `internal/convert/` (validators, utility functions), but that dependency is addressed separately in `migrate-helm-to-library-olm`.

After this migration, `internal/convert/` retains only the utility functions and validator used by `internal/helm/` until the helm migration completes.
