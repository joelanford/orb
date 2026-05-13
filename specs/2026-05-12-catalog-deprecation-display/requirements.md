# Requirements

- Bump library-olm to at least commit `24b3d25` to pick up the `Result` struct and `PreferNonDeprecatedBundles` resolver option
- `catalog info` shows a Deprecations section listing all deprecation entries (package, channel, bundle) with their messages
- `catalog search` hides deprecated packages by default; `--include-deprecated` shows them with `†` marker and dimmed styling
- `catalog resolve` hides deprecated bundles by default; `--include-deprecated` shows them with `†` marker and dimmed styling
- `catalog resolve` adds `deprecated` (bool) and `deprecationMessage` (string, omitempty) fields to JSON/YAML output; deprecated bundles filtered unless `--include-deprecated`
- `catalog resolve` uses `PreferNonDeprecatedBundles()` by default
- Helm plugin orb-getter uses `PreferNonDeprecatedBundles()` to prefer non-deprecated bundles
- `catalog resolve` callers updated to use the new `*Result` return type
- Existing tests continue to pass after the library-olm bump and Resolve signature change

## Acceptance Criteria

- `orb catalog info <deprecated-pkg>` displays deprecation messages for the package, its deprecated channels, and its deprecated bundles
- `orb catalog info <non-deprecated-pkg>` shows no Deprecations section
- `orb catalog search` hides deprecated packages; `--include-deprecated` shows them with `†` marker, dimmed, and legend
- `orb catalog resolve <pkg>` hides deprecated bundles; `--include-deprecated` shows them with `†` marker, dimmed, and legend
- `orb catalog resolve <pkg> -o json --include-deprecated` includes `"deprecated": true` and `"deprecationMessage": "..."` for deprecated bundles
- Non-deprecated bundles sort before deprecated bundles when `--include-deprecated` is used
- `make test`, `make lint`, and `make verify` all pass
