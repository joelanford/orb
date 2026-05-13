---
status: pr-submitted
pr: https://github.com/joelanford/orb/pull/39
---
# Surface deprecation status in catalog commands

## Summary

Display deprecation information for packages, channels, and bundles across catalog commands. library-olm stores deprecation data via `catalogv1.Deprecated` (a type-assertion interface on `UpdateGraph` and `Bundle` values) — orb previously ignored it entirely.

This work item bumps library-olm to pick up the `Result` struct from `Resolve()` (which includes the `Package` field) and the `PreferNonDeprecatedBundles()` resolver option, then surfaces deprecation across the three query commands.

## Design

### library-olm bump

Bump to at least commit `24b3d25` which includes:
- `resolverv1.Result` struct (fields: `Catalog`, `Package`, `Bundles`) replacing the previous `(Catalog, []Bundle, error)` return
- `resolverv1.PreferNonDeprecatedBundles()` option — sorts non-deprecated bundles before deprecated ones
- All deprecation query support (already in the current version)

The `Resolve()` signature change from `(catalogv1.Catalog, []bundlev1.Bundle, error)` to `(*Result, error)` requires updating `runCatalogResolve` and `printResolveResults` callers.

### Deprecation detection pattern

All three commands use the same type-assertion pattern:

```go
if d, ok := value.(catalogv1.Deprecated); ok {
    msg := d.DeprecationMessage()
    // use msg
}
```

Where `value` is an `UpdateGraph` (package or channel) or `bundlev1.Bundle`.

### `catalog info` — deprecations section

After the existing fields (Package, Display Name, Catalog, Description, Keywords, Channels, Versions), add a **Deprecations** section that lists all deprecation entries for the package. Each entry shows what is deprecated and its message:

```
Deprecations:
  Package: this package is deprecated, use new-pkg instead
  Channel stable: this channel is deprecated
  Bundle foo.v1.0.0: this bundle version has a known CVE
```

Only show the section if at least one deprecation exists. This is the detail view — always show deprecations when present (no filtering).

### `catalog search` — hide deprecated, footnote marker

Deprecated packages are **hidden by default**. Use `--include-deprecated` to show them.

When shown, deprecated packages are marked with `†` appended to the package name, dimmed via `Faint(true)`, and a legend at the bottom explains the marker:

```
PACKAGE       DISPLAY NAME     CATALOG
vault         HashiCorp Vault  operatorhubio
old-operator† Old Operator     operatorhubio

† = deprecated
```

No extra column — keeps the table compact since deprecation is sparse.

### `catalog resolve` — hide deprecated, footnote marker

Deprecated bundles are **hidden by default**. Use `--include-deprecated` to show them.

**Table output:** When shown, deprecated bundles are marked with `†` appended to the version, dimmed via `Faint(true)`, and a legend at the bottom:

```
CATALOG        VERSION  IMAGE
operatorhubio  1.2.0    quay.io/foo/bundle:v1.2.0
operatorhubio  1.1.0    quay.io/foo/bundle:v1.1.0
operatorhubio  1.0.0†   quay.io/foo/bundle:v1.0.0

† = deprecated
```

**Structured output (JSON/YAML):** `deprecated` boolean field (always present) and `deprecationMessage` string field (`omitempty`). Deprecated bundles are also filtered out unless `--include-deprecated` is set.

**Default sort order:** `resolverv1.PreferNonDeprecatedBundles()` is always passed so that when `--include-deprecated` is used, non-deprecated bundles appear first.

### Helm plugin (`helm-plugin run orb-getter`)

The orb-getter Helm plugin also calls `resolverv1.Resolve()`. It uses `PreferNonDeprecatedBundles()` so that non-deprecated bundles are preferred when multiple candidates match. No filtering or UI markers are needed here — the plugin always picks the single best bundle and outputs a chart archive.

### No changes needed

- `catalog list` — catalogs themselves don't have deprecation
- `catalog add` / `catalog update` / `catalog remove` / `catalog edit` — mutation commands, not query
- Schema migration — library-olm handles this automatically on `OpenStore` when the schema version changes
