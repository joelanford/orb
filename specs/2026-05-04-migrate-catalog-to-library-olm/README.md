---
status: in-progress
---
# Migrate catalog layer to library-olm

## Summary

Replace orb's catalog layer with library-olm's `catalog/v1`, `catalog/fbc`, and `resolver/v1` packages. After this migration, orb's catalog code is a thin CLI layer that delegates all FBC parsing, storage, upgrade graph computation, resolution, query operations, and display metadata storage to library-olm. Display metadata (displayName, description, keywords, icon) and related images are stored as library-olm properties via the FBC extension interface.

## Dependencies

- library-olm persistent catalog DB (merged to `main`)
- library-olm predecessor ranges (merged to `main`)
- library-olm resolver/v1 with StoreReader, Select, and BundleIdentity (merged to `main`)
- library-olm properties and FBC extensions (PR #6, pending)
- `migrate-convert-to-library-olm` (done, PR #36)

## Design

### Store and FBC import

Orb's `catalog.DB` is replaced by library-olm's `catalogv1.Store`. The `catalogv1.OpenStore(path)` function opens (or creates) a SQLite-backed store. All CRUD operations (`Set`, `Get`, `Delete`, `List`) are delegated to library-olm.

FBC import uses `fbc.NewImporter(fs.FS, fbc.WithOLMPackageExtension(ext))` passed to `store.Set(ctx, name, WithContent(importer, digest))`. The importer handles FBC parsing, normalization, version extraction, channel/bundle/entry construction, and display metadata extraction via the extension.

Old schema migration runs on first open when legacy `catalogs`/`packages` tables are detected.

### Display metadata via FBC extension

Library-olm provides first-class support for custom metadata on catalog entities:

- **Properties storage**: `content_bundle_properties` and `content_graph_properties` tables with cascade-delete semantics. Properties are part of the content layer — dropped and rebuilt on schema version mismatch.
- **Writer API**: `SetBundleProperty(bundleID, key, val)` and `SetGraphProperty(path, key, val)` on `Writer`.
- **Reading API**: `Property(ctx, key)` on `Bundle` and `UpdateGraph` interfaces. Returns `(nil, nil)` when key is not found.
- **FBC Extension interface**: `OLMPackageExtension` with per-blob callbacks (`OnPackage`, `OnChannel`, `OnBundle`, `OnDeprecation`, `OnOther`) and a `FinalizePackage` step. Extension data is stored in staging tables and accessible via `PackageAccessor` during finalization. Properties written via `PropertyWriter` are readable through the standard `Bundle.Property`/`UpdateGraph.Property` API after import completes.
- **PropertyWriter**: Two methods — `SetBundleProperty(ctx, bundleName, key, val)` and `SetGraphProperty(ctx, path, key, val)`. The `path` is relative to the package root: `[]string{}` for the package graph, `[]string{"channelName"}` for a channel graph. The implementation prepends the package name internally.
- **Graceful error handling**: `FinalizePackage` errors are per-package and appear in `PartialImportError` without affecting other packages.

#### Property key mapping

| Display field | Property key | Scope | Value type |
|---|---|---|---|
| `displayName` | `orb.displayName` | package (graph path `[]string{}`) | `string` |
| `description` | `orb.description` | package (graph path `[]string{}`) | `string` |
| `keywords` | `orb.keywords` | package (graph path `[]string{}`) | `[]string` |
| `icon` | `orb.icon` | package (graph path `[]string{}`) | `{"data":"...","mediaType":"..."}` |
| `relatedImages` | `orb.relatedImages` | bundle | `[]string` |

Package-level metadata (displayName, description, keywords, icon) is stored as graph properties with an empty path (`[]string{}`), which the `PropertyWriter` resolves to the package graph internally. Bundle-level metadata (relatedImages) is stored as bundle properties.

#### Extension implementation

Orb implements `fbc.OLMPackageExtension`:

- **`OnPackage`**: Extract `description` and `icon` from `declcfg.Package`. Return as ext_data for `FinalizePackage`.
- **`OnBundle`**: Extract `displayName`, `description`, and `keywords` from the `olm.csv.metadata` property (if present), and collect image refs from `relatedImages`. Version and release are already in the raw table via `BundleAccessor`. Return as ext_data for `FinalizePackage`.
- **`OnChannel`/`OnDeprecation`/`OnOther`**: Return `nil, nil` (no data needed).
- **`FinalizePackage`**: Read ext_data from `PackageAccessor`. Find the highest-version bundle (using `BundleAccessor.Version()`/`Release()` from the raw table). Assemble package-level metadata with these precedence rules:
  - `orb.displayName` — from highest-version bundle's csv metadata
  - `orb.description` — from package blob (`OnPackage` ext_data); falls back to highest-version bundle's csv metadata only if package description is empty
  - `orb.keywords` — from highest-version bundle's csv metadata (sorted, deduplicated)
  - `orb.icon` — from package blob (`OnPackage` ext_data) only
  
  Write package-level properties via `SetGraphProperty(ctx, []string{}, key, val)`. Write per-bundle `orb.relatedImages` via `SetBundleProperty`.

#### Reading properties

- `orb catalog info PACKAGE`: Get the package's `UpdateGraph` (via `cat.GetPackage`), call `Property(ctx, "orb.displayName")`, etc.
- `orb catalog search KEYWORD`: Iterate packages, read `orb.keywords`, `orb.displayName`, and `orb.description` properties.
- Bundle related images: Call `bundle.Property(ctx, "orb.relatedImages")`.

### Resolve via resolver/v1

Library-olm provides `resolver/v1.Resolve()` which handles:
- Priority-based catalog selection (highest priority first, with ambiguity detection when multiple catalogs at the same priority contain the package)
- Graph path walking via `WithGraphs([][]string{...})` (replaces orb's channel filtering)
- Version constraint filtering via `WithMastermindsVersionConstraint()` (replaces orb's `mastermindsConstraintToBlangRange`)
- Successor filtering via `WithSuccessorsOf(BundleIdentity)` (replaces orb's manual successor collection)
- Deduplication and version-descending sort of results

Library-olm also provides:
- `StoreReader` interface — read-only subset of `Store` with `Get`, `List`, and `Select`
- `Store.Select(labels.Selector)` — returns a `StoreReader` filtered by label selector (library-olm injects `olm.operatorframework.io/metadata.name` as a synthetic label, matching orb's previous manual behavior)
- `BundleIdentity` interface — `Successors()` takes `BundleIdentity` (ID + NameVersionRelease) instead of bare `BundleID`, enabling predecessor range evaluation at query time
- `Writer.AddEdge()` replaces `AddSuccessor()`
- `Writer.AddPredecessorRange()` — stores skipRange as a predecessor range evaluated at query time instead of expanding into explicit edges at ingest

#### API mapping

| orb resolve.go (remove) | library-olm resolver/v1 (use instead) |
|---|---|
| `Resolve(ctx, store, pkg, opts)` | `resolverv1.Resolve(ctx, reader, pkg, ...ResolveOption)` |
| `opts.Channels` → `selectGraphs()` | `resolverv1.WithGraphs([][]string{{"ch1"}, {"ch2"}})` |
| `opts.Version` → `mastermindsConstraintToBlangRange()` | `resolverv1.WithMastermindsVersionConstraint(constraint)` |
| `opts.InstalledBundleID` → `collectCandidates()` + `g.Successors()` | `resolverv1.WithSuccessorsOf(bundleIdentity)` |
| `opts.CatalogLabelSelector` → manual label matching | `store.Select(selector)` → pass `StoreReader` to `Resolve` |
| `resolveCandidate` type + `buildResults()` | `resolverv1.Resolve` returns `(Catalog, []Bundle, error)` |

#### Resolve output changes

The resolver returns `(catalogv1.Catalog, []bundlev1.Bundle, error)` instead of `[]ResolveResult`. orb's CLI formatting code converts these to the output format. Key differences:

- The resolver returns the winning `Catalog` directly (orb previously threaded catalog name through `ResolveResult`)
- Bundles are `bundlev1.Bundle` interface values (ID, NameVersionRelease, URI)
- Channel membership is no longer part of the result — bundles are returned flat. If the user filtered by channel, only matching bundles appear. The `channels` field in JSON/YAML output is removed (breaking change, acceptable per project policy)

#### Installed bundle identity

The `--installed` flag uses `name=version` syntax (e.g., `--installed vault-operator.v0.4.10=0.4.10`), matching older orb versions. The value is parsed with `strings.Cut(value, "=")` into a bundle ID and version string. The version is parsed via `bsemver.Parse()` to construct a `BundleIdentity` with both the ID (for explicit edge matching) and the version (for predecessor range evaluation). This is passed to `resolverv1.WithSuccessorsOf(identity)`.

#### Search, info, and list

These commands use `catalogv1.Store` directly. Display metadata is read via the `Property()` API on `UpdateGraph` and `Bundle`. The `Select` API is not needed here because search/info don't filter by catalog labels.

### What gets deleted from orb

- `internal/catalog/resolve.go` — entirely replaced by `resolverv1.Resolve()`
- `internal/catalog/metadata.go` — replaced by library-olm properties
- `internal/catalog/extract.go` — replaced by `OLMPackageExtension`
- `internal/catalog/migrate.go` — no longer needed
- The `ResolveOptions`, `ResolveResult`, `resolveCandidate` types
- `selectGraphs()`, `collectCandidates()`, `buildResults()`, `resolveFromPackage()`
- `mastermindsConstraintToBlangRange()` — library-olm's resolver handles this internally

### What stays in orb

- `internal/catalog/config.go` — `DefaultDBPath()` (CLI-specific path resolution)
- `internal/catalog/extension.go` — `OLMPackageExtension` implementation for display metadata
- `internal/cmd/catalog.go` — CLI commands, adapted to call `resolverv1.Resolve()` and read properties

### Dependency changes

- `operator-framework/operator-registry` dependency remains — orb's extension implementation needs the `declcfg` types for the `OLMPackageExtension` interface
- `samber/lo` dependency is removed — no remaining usages after resolve and extract deletion
