---
status: in-progress
---
# Migrate plain manifest conversion to library-olm

## Summary

Replace orb's `internal/convert/` package and `internal/bundle/` RegistryV1 type with library-olm's `bundle/registry/v1` package. This eliminates ~4250 lines of duplicated conversion logic (validators, resource generators, cert providers, utilities) by using library-olm's `ToPlainManifests()` API. Aligns with the "thin wrapper over library-olm" design principle.

## Design

### Bundle type migration

orb's `bundle.RegistryV1` struct is structurally identical to library-olm's `registryv1.Bundle`. All code that imports `internal/bundle` for the `RegistryV1` type switches to `registryv1.Bundle` directly — no type aliases or boundary conversions.

`internal/bundle/bundle.go` (the `RegistryV1` struct) and `internal/bundle/fromfs.go` (`FromFS` function) are deleted. Their library-olm equivalents:

| orb (remove) | library-olm (use instead) |
|---|---|
| `bundle.RegistryV1` | `registryv1.Bundle` |
| `bundle.FromFS(fs.FS) (*RegistryV1, error)` | `registryv1.FromFS(fs.FS) (Bundle, error)` |

Note the pointer-to-value change: library-olm returns `Bundle` by value. Callers passing `*bundle.RegistryV1` switch to `*registryv1.Bundle` or value as appropriate.

`internal/bundle/` retains `registry.go` (vendored operator-registry types used by catalog code), `versionrelease.go` (orb-specific version parsing), and their tests.

### Conversion API migration

All `convert.Converter.Convert()` calls are replaced with `registryv1.ToPlainManifests()`:

| orb (remove) | library-olm (use instead) |
|---|---|
| `convert.Converter.Convert(rv1, ns, opts...)` | `registryv1.ToPlainManifests(b, ns, opts...)` |
| `convert.Option` | `registryv1.RenderOption` |
| `convert.WithTargetNamespaces(ns)` | `registryv1.WithTargetNamespaces(ns)` |
| `convert.WithCertificateProvider(cp)` | `registryv1.WithCertificateProvider(cp)` |
| `convert.CertificateProvider` | `registryv1.CertificateProvider` |

### Cert provider migration

orb's `internal/convert/certproviders/` (CertManagerCertificateProvider, OpenshiftServiceCaCertificateProvider) are deleted. Requires a library-olm change to export these implementations from a public package (currently in library-olm's `internal/`).

The `getCertificateProvider()` function in `internal/cmd/bundle.go` switches to library-olm's exported cert provider types.

### Helm package — retained convert dependencies

`internal/helm/` continues to use `internal/convert/` for:
- `convert.Converter.BundleValidator.Validate(b)` — bundle validation before chart generation
- `convert.MergeMaps(...)` — annotation merging in deployment generation
- `convert.ObjectNameForBaseAndSuffix(...)` — cert/service naming
- `convert.DefaultUniqueNameGenerator(...)` — deterministic RBAC naming

These dependencies are addressed in the separate `migrate-helm-to-library-olm` work item. For this migration, `internal/convert/` is pruned to retain only the types and functions helm needs.

### Callers to update

1. **`internal/cmd/bundle.go`** — replace `convert.Option`, `convert.WithTargetNamespaces`, `convert.WithCertificateProvider`, and `certproviders.*` with library-olm equivalents
2. **`internal/destination/destination.go`** — change `ConvertOpts []convert.Option` to `[]registryv1.RenderOption`
3. **`internal/destination/plain.go`** — replace `convert.Converter.Convert()` with `registryv1.ToPlainManifests()`
4. **`internal/source/regv1.go`** — replace `bundle.FromFS()` with `registryv1.FromFS()`; update return types
5. **`internal/source/source.go`** — update `Source` interface to use `registryv1.Bundle`
6. **All `internal/helm/*.go`** — update `*bundle.RegistryV1` parameter types to `*registryv1.Bundle`
7. **All `internal/destination/helm*.go`** — update `*bundle.RegistryV1` parameter types
8. **`internal/catalog/fbc.go`** — update embedded `bundle.RegistryV1` references (note: catalog embeds `VersionRelease`, not `RegistryV1` directly, so this may not need changes)

### What gets deleted

- `internal/convert/convert.go` — `BundleConverter`, `Convert()`, `ResourceGenerator`, `ResourceGenerators`, `BundleValidator`, `Options`, `Option` functions (partially — keep what helm needs)
- `internal/convert/generators.go` — all 10 resource generators (521 lines)
- `internal/convert/validators.go` — all 12 validators (306 lines) (partially — keep `BundleValidator` and validators for helm)
- `internal/convert/resources.go` — resource builder functions (244 lines)
- `internal/convert/registryv1.go` — pre-wired `Converter` instance (partially — keep for helm)
- `internal/convert/certprovider.go` — `CertificateProvider` interface and related types
- `internal/convert/certproviders/` — both cert provider implementations
- `internal/convert/util.go` — utility functions (partially — keep for helm)
- `internal/bundle/bundle.go` — `RegistryV1` struct definition
- `internal/bundle/fromfs.go` — `FromFS` function and `RegistryV1Properties` type
- All corresponding test files for deleted code
