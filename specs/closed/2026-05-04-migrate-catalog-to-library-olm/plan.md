# Implementation Plan

1. **Update library-olm dependency**
   - `go get github.com/joelanford/library-olm@main` (needs persistent catalog DB, predecessor ranges, resolver/v1, properties + FBC extensions)
   - `go mod tidy`

2. **Rewrite catalog store integration**
   - Replace `catalog.DB` with `catalogv1.OpenStore(path)`
   - Wire all CRUD operations (`add`, `remove`, `list`, `edit`, `update`) through `catalogv1.Store`

3. **Implement `OLMPackageExtension` for display metadata**
   - Create `internal/catalog/extension.go`
   - `OnPackage`: extract description and icon from `declcfg.Package`, return as ext_data
   - `OnBundle`: extract `displayName`, `description`, `keywords` from `olm.csv.metadata` (if present), and collect image refs from `relatedImages`, return as ext_data (version/release already in raw table via `BundleAccessor`)
   - `OnChannel`/`OnDeprecation`/`OnOther`: return `nil, nil`
   - `FinalizePackage`: read ext_data from `PackageAccessor`, find highest-version bundle, write package-level properties (`orb.displayName`, `orb.description`, `orb.keywords`, `orb.icon`) via `SetGraphProperty(ctx, []string{}, key, val)`, write per-bundle `orb.relatedImages` via `SetBundleProperty`

4. **Wire FBC import with extension**
   - Use `fbc.NewImporter(fsys, fbc.WithOLMPackageExtension(ext))` in catalog add/update
   - Pass importer to `store.Set(ctx, name, WithContent(importer, digest))`

5. **Rewrite `catalog info` and `catalog search` to read properties**
   - `catalog info`: get `UpdateGraph` via `cat.GetPackage`, read `orb.displayName`, `orb.description`, `orb.keywords`, `orb.icon` via `Property(ctx, key)`
   - `catalog search`: iterate packages, read `orb.keywords`, `orb.displayName`, `orb.description` properties
   - Related images: read `orb.relatedImages` via `bundle.Property(ctx, "orb.relatedImages")`

6. **Rewrite `catalog resolve` using resolver/v1**
   - Parse `--catalog-label-selector` via `labels.Parse()`, call `store.Select(selector)` to get a `StoreReader`
   - Parse `--version` via `mmsemver.NewConstraint()`, wrap with `resolverv1.WithMastermindsVersionConstraint()`
   - Map `--channel` values to graph paths: each channel name becomes a single-element path `[]string{ch}`, collected into `[][]string` and passed to `resolverv1.WithGraphs()`
   - Handle `--installed`: parse `name=version` with `strings.Cut`, construct `BundleIdentity`, pass via `resolverv1.WithSuccessorsOf()`
   - Call `resolverv1.Resolve(ctx, reader, packageName, ...opts)`
   - Handle nil catalog (package not found) — return user-friendly error
   - Convert `(Catalog, []Bundle)` to CLI output format

7. **Update resolve output types and formatting**
   - Define a thin output struct for JSON/YAML serialization (catalog, bundleID, version, release, uri)
   - Remove the `channels` field (breaking change)
   - Update table output columns: CATALOG, VERSION, IMAGE
   - Update `printResolveResults` to accept the new types
   - JSONPath output applies to the same structure

8. **Delete replaced code**
   - Delete `internal/catalog/resolve.go` (resolve logic)
   - Delete `internal/catalog/metadata.go` and `internal/catalog/metadata_test.go` (MetadataDB)
   - Delete `internal/catalog/extract.go` and `internal/catalog/extract_test.go` (ExtractDisplayMetadata)
   - Delete `internal/catalog/migrate.go` and `internal/catalog/migrate_test.go` (old schema migration)

9. **Remove `samber/lo` dependency**
   - `resolve.go` (deleted) and `extract.go` (deleted) are the only usages
   - Remove `samber/lo` from `go.mod`
   - Run `go mod tidy`

10. **Update tests**
    - Add tests for the `OLMPackageExtension` implementation
    - Update integration tests to verify properties are readable after import
    - Remove tests that depend on `MetadataDB`

11. **Update CLAUDE.md and tech-stack.md**
    - Add `resolver/v1` to library-olm's purpose in the dependencies table
    - Update `internal/catalog/` description to reflect extension-based metadata
    - Add `fbc.OLMPackageExtension` to library-olm integration description

12. **Final verification**
    - `make test`, `make lint`, `make verify` all pass
    - Manual smoke test: `orb catalog add`, `orb catalog info`, `orb catalog search` verify display metadata
    - Manual smoke test: `orb catalog resolve` with combinations of `--channel`, `--version`, `--installed`, `-l`, `-o json`, `-o yaml`
    - Confirm `orb_*` tables do not exist in newly created databases
