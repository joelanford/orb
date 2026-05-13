# Verification

## Implementation Correctness

- [ ] library-olm bumped to >= `24b3d25`; `go.mod` and `go.sum` updated
- [ ] `runCatalogResolve` uses `*resolverv1.Result` return type correctly (nil check, field access)
- [ ] `resolverv1.PreferNonDeprecatedBundles()` passed unconditionally in resolve
- [ ] `resolveResultItem` has `deprecated` and `deprecationMessage` fields with correct JSON/YAML tags
- [ ] `catalog search` hides deprecated packages by default; `--include-deprecated` shows them
- [ ] `catalog search` uses `†` footnote marker and dimmed styling for deprecated packages
- [ ] `catalog resolve` hides deprecated bundles by default; `--include-deprecated` shows them
- [ ] `catalog resolve` uses `†` footnote marker and dimmed styling for deprecated bundles
- [ ] `catalog info` shows Deprecations section only when deprecations exist
- [ ] `catalog info` lists package-level, channel-level, and bundle-level deprecations with messages
- [ ] All deprecation checks use `catalogv1.Deprecated` type assertion (not reflection or string checks)
- [ ] Legend displayed at bottom of table when deprecated entries are shown

## Project Conventions

- [ ] Commits use conventional commit format (`feat:`, `chore:`)
- [ ] No exported APIs added outside `internal/`
- [ ] Core OLM logic delegated to library-olm, not reimplemented in orb (mission.md principle 6)
- [ ] Terminal styling uses lipgloss (tech-stack.md)
- [ ] `make test` passes
- [ ] `make lint` passes
- [ ] `make verify` passes (no uncommitted changes after tidy/lint-fix)
