# Implementation Plan

1. **Library-olm prerequisite: export cert providers**
   - Add a public package in library-olm that exports `CertManagerCertificateProvider` and `OpenshiftServiceCaCertificateProvider`
   - Release a new library-olm version
   - Update orb's `go.mod` to use the new version

2. **Replace bundle type throughout**
   - Delete `internal/bundle/bundle.go` and `internal/bundle/fromfs.go`
   - Update `internal/source/source.go` interface to return `registryv1.Bundle`
   - Update all `internal/source/regv1.go` functions to use `registryv1.FromFS()` and return `registryv1.Bundle`
   - Update `internal/destination/destination.go` interface to accept `registryv1.Bundle`
   - Update all destination implementations (`plain.go`, `helm.go`) parameter types
   - Update all `internal/helm/*.go` parameter types
   - Run `make lint-fix` to sort imports

3. **Replace conversion calls**
   - Change `internal/destination/destination.go`: `ConvertOpts []convert.Option` → `[]registryv1.RenderOption`
   - Change `internal/destination/plain.go`: replace `convert.Converter.Convert()` with `registryv1.ToPlainManifests()`
   - Change `internal/cmd/bundle.go`: replace `convert.Option`, `convert.WithTargetNamespaces`, `convert.WithCertificateProvider` with library-olm equivalents
   - Change `internal/cmd/bundle.go`: replace `certproviders.*` with library-olm's exported cert provider types
   - Delete `internal/convert/certproviders/` directory
   - Delete `internal/convert/certprovider.go`

4. **Prune internal/convert/**
   - Delete `internal/convert/generators.go` and `generators_test.go`
   - Delete `internal/convert/resources.go` and `resources_test.go`
   - Prune `internal/convert/convert.go` to retain only `BundleValidator`, `Validate()`, and the types/functions `internal/helm/` still needs
   - Prune `internal/convert/validators.go` to retain only what `BundleValidator` references
   - Prune `internal/convert/registryv1.go` to retain only the wired-up validator
   - Retain `internal/convert/util.go` (used by helm)
   - Delete test files for removed code; keep tests for retained code

5. **Verify behavioral equivalence**
   - Run `make test`, `make lint`, `make verify`
   - Spot-check `orb bundle convert plain` with a real bundle image to confirm identical output
