# Requirements

## Functional

- All catalog CRUD operations (add, remove, list, edit, update) use library-olm's `catalogv1.Store` instead of orb's `catalog.DB`
- FBC content import uses `fbc.NewImporter` + `catalogv1.WithContent` with an `OLMPackageExtension` for display metadata
- Display metadata (displayName, description, keywords, icon) is stored as graph properties via `SetGraphProperty(ctx, []string{}, key, val)` (empty path = package root)
- Bundle related images are stored as bundle properties
- Properties are written during FBC import via `FinalizePackage` — no separate post-import walk
- Properties participate in library-olm's cascade-delete — no manual cleanup needed on `catalog remove`
- Properties participate in content schema version rebuild — no separate migration logic needed
- `orb catalog search` keyword matching reads `orb.keywords`, `orb.displayName`, `orb.description` properties
- `orb catalog info` reads display metadata from `UpdateGraph.Property()` and `Bundle.Property()`
- Old orb databases (with `catalogs` and `packages` tables) are migrated on first open
- Replace `internal/catalog/resolve.go` with calls to `resolverv1.Resolve()` from library-olm's `resolver/v1` package
- Use `store.Select(labels.Selector)` for catalog label filtering instead of manual matching in resolve
- Map `--channel` values to graph paths via `resolverv1.WithGraphs()`
- Map `--version` constraint to `resolverv1.WithMastermindsVersionConstraint()`
- Construct `BundleIdentity` for `--installed` flag and pass via `resolverv1.WithSuccessorsOf()`
- Convert resolver output (`Catalog`, `[]Bundle`) to CLI output formats (table, JSON, YAML, jsonpath)
- Remove the `channels` field from resolve JSON/YAML output (breaking change, acceptable per project policy)
- The `operator-framework/operator-registry` dependency remains for the `OLMPackageExtension` interface types

## Deletions

- `internal/catalog/resolve.go` — entirely replaced by `resolverv1.Resolve()`
- `internal/catalog/metadata.go` — replaced by library-olm properties
- `internal/catalog/extract.go` — replaced by `OLMPackageExtension`
- `internal/catalog/migrate.go` — no longer needed
- `samber/lo` dependency — no remaining usages

## Acceptance Criteria

- `orb catalog add NAME docker://REF` creates a catalog, imports FBC content via library-olm, and populates display metadata as properties
- `orb catalog update` re-pulls changed catalogs, updates library-olm content and display metadata properties, skips unchanged catalogs by digest
- `orb catalog remove NAME` deletes library-olm content (properties cascade-deleted automatically)
- `orb catalog edit NAME --priority N --label k=v` updates catalog metadata via `store.Set`
- `orb catalog list` shows all catalogs sorted by priority descending
- `orb catalog search KEYWORD` matches against displayName, description, keywords from properties
- `orb catalog info PACKAGE` displays displayName, description, keywords, icon, channels, and bundle count
- `orb catalog resolve PACKAGE` returns bundles from the highest-priority catalog containing the package, sorted by version descending
- `orb catalog resolve PACKAGE --channel <ch>` filters to bundles in the specified channel
- `orb catalog resolve PACKAGE --version ">=1.0"` filters by semver constraint
- `orb catalog resolve PACKAGE --installed <bundleID>` returns only successor bundles
- `orb catalog resolve PACKAGE -l env=prod` uses `Select` to filter catalogs by label
- When two catalogs at the same priority both contain the package, resolve returns an ambiguity error
- `orb catalog resolve PACKAGE -o json` produces valid JSON output
- `make test`, `make lint`, and `make verify` all pass
- `internal/catalog/resolve.go` is deleted
- `internal/catalog/metadata.go`, `internal/catalog/extract.go`, and `internal/catalog/migrate.go` are deleted
- `orb_*` tables do not exist in newly created databases
- Display metadata is readable via `Property()` API after catalog import
- `FinalizePackage` errors for one package do not prevent other packages from importing
