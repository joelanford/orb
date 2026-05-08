# Verification

## Store and FBC import
- [ ] `catalogv1.OpenStore` is called with the path from `DefaultDBPath()`
- [ ] `fbc.NewImporter(os.DirFS(tmpDir), fbc.WithOLMPackageExtension(ext))` is used for FBC content import
- [ ] `fbc.PartialImportError` is handled (partial imports commit successfully)
- [ ] Parallel catalog update (`errgroup` with worker limit) works with library-olm's Store
- [ ] Old schema migration detects `catalogs` table, copies metadata, drops old tables

## Display metadata via FBC extension
- [ ] `OLMPackageExtension` implementation extracts displayName, description, keywords, icon from FBC package and bundle blobs
- [ ] `OnPackage` captures description and icon data as ext_data
- [ ] `OnBundle` captures `displayName`, `description`, `keywords` from `olm.csv.metadata` and image refs from `relatedImages` as ext_data (does not re-extract version/release — already in raw table)
- [ ] `OnChannel`/`OnDeprecation`/`OnOther` return `nil, nil`
- [ ] `FinalizePackage` finds highest-version bundle and writes package-level display metadata
- [ ] Package properties use `SetGraphProperty(ctx, []string{}, key, val)` (empty path = package root)
- [ ] Bundle properties use `SetBundleProperty(ctx, bundleName, key, val)`
- [ ] Property keys use `orb.` prefix (`orb.displayName`, `orb.description`, `orb.keywords`, `orb.icon`, `orb.relatedImages`)
- [ ] `orb catalog info` reads metadata from `UpdateGraph.Property()`
- [ ] `orb catalog search` reads keywords/displayName/description from properties
- [ ] Bundle related images read from `Bundle.Property()`
- [ ] `catalog remove` does not need explicit metadata cleanup — library-olm cascade-delete handles it
- [ ] `FinalizePackage` per-package errors do not block other packages
- [ ] Extension is registered via `fbc.WithOLMPackageExtension(ext)` option on `NewImporter`

## Resolver migration
- [ ] `resolverv1.Resolve()` is used instead of orb's `catalog.Resolve()`
- [ ] `store.Select(selector)` replaces manual label matching in resolve — library-olm injects `olm.operatorframework.io/metadata.name` automatically
- [ ] `--channel` values are mapped to `WithGraphs([][]string{{"ch1"}, {"ch2"}})` (each channel is a single-element path)
- [ ] `--version` is parsed via `mmsemver.NewConstraint()` and passed to `WithMastermindsVersionConstraint()`
- [ ] `--installed` accepts `name=version` syntax, parses into `BundleIdentity`, and passes to `WithSuccessorsOf()`
- [ ] Ambiguity errors (same-priority catalogs) surface as user-friendly CLI errors
- [ ] Nil catalog result (package not found) returns a clear error message
- [ ] JSON/YAML output no longer includes a `channels` field

## Deletions
- [ ] `internal/catalog/resolve.go` is fully deleted — no remnant types or functions
- [ ] `internal/catalog/metadata.go` fully deleted — no `MetadataDB` type remains
- [ ] `internal/catalog/extract.go` fully deleted — no `ExtractDisplayMetadata` remains
- [ ] `internal/catalog/migrate.go` fully deleted — no old schema migration remains
- [ ] No `orb_*` tables created in new databases
- [ ] `samber/lo` dependency removed

## Project conventions
- [ ] Commit messages follow conventional commits format
- [ ] No `//nolint` comments added
- [ ] All new code is in `internal/` — no exported APIs
- [ ] `make test` passes with `-race -count=1`
- [ ] `make lint` passes
- [ ] `make verify` passes (tidy + lint-fix + diff check)
- [ ] Deleted code has no remaining import references
- [ ] `go.mod` is tidy — no unused dependencies
- [ ] Design aligns with mission.md principle 6: "Thin wrapper over library-olm"
- [ ] CLAUDE.md and tech stack doc updated
