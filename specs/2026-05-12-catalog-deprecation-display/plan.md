# Implementation Plan

1. **Bump library-olm dependency**
   - `go get github.com/joelanford/library-olm@24b3d25`
   - `go mod tidy`
   - Fix compilation: update `runCatalogResolve` to use the new `*Result` return type (`result.Catalog`, `result.Bundles` instead of tuple destructuring; handle `nil` result for not-found)
   - Verify `make build` and `make test` pass with the new dependency

2. **Add deprecation to `catalog resolve`**
   - Add `deprecated` (bool) and `deprecationMessage` (string, omitempty) to `resolveResultItem`
   - In `printResolveResults`, type-assert each bundle to `catalogv1.Deprecated` and populate the new fields
   - Filter deprecated bundles by default; add `--include-deprecated` flag to opt in
   - Add `†` footnote marker and dimmed styling for deprecated bundles in table output
   - Pass `resolverv1.PreferNonDeprecatedBundles()` unconditionally so non-deprecated sort first

3. **Add `PreferNonDeprecatedBundles()` to helm plugin**
   - In `runHelmPluginOrbGetter`, add `resolverv1.PreferNonDeprecatedBundles()` to the resolve options so the plugin prefers non-deprecated bundles

4. **Add deprecation to `catalog search`**
   - Add `deprecated bool` field to the `searchResult` struct
   - In `runCatalogSearch`, type-assert each package `UpdateGraph` to `catalogv1.Deprecated`
   - Filter deprecated packages by default; add `--include-deprecated` flag to opt in
   - Add `†` footnote marker and dimmed styling for deprecated packages in table output
   - Add combined legend for `*` (shadowed) and `†` (deprecated) markers

5. **Add deprecation to `catalog info`**
   - In `runCatalogInfo`, after existing fields, collect deprecation entries:
     - Type-assert package `UpdateGraph` to `catalogv1.Deprecated`
     - Iterate channels via `CompositeUpdateGraph.ListGraphs`, type-assert each to `catalogv1.Deprecated`
     - Iterate bundles via `ListBundles`, type-assert each to `catalogv1.Deprecated`
   - If any deprecations found, print a `Deprecations:` section with indented entries

6. **Verify**
   - `make test` — all existing + any new tests pass
   - `make lint` — no lint issues
   - `make verify` — no uncommitted changes after tidy/lint-fix
