---
status: in-progress
---
# Update CI and linting

## Summary

Modernize the CI pipeline and align the golangci-lint configuration with library-olm's conventions. Remove govulncheck (redundant with dependabot, soft-blocks unrelated PRs), add dependabot for automated dependency updates across three ecosystems, and update the linter set to match library-olm while keeping orb-specific linters that add value.

## Design

### govulncheck removal

Remove the `vulncheck` Makefile target and the `vulncheck` CI job. Dependabot's vulnerability alerts cover the same ground without blocking PRs that touch unrelated code. The `go tool govulncheck` tool dependency can also be removed from `go.mod` if present.

### Dependabot configuration

Add `.github/dependabot.yml` covering:

1. **Go modules** (`gomod`) — weekly updates for `go.mod`
2. **GitHub Actions** (`github-actions`) — weekly updates for action versions in `.github/workflows/`
3. **Docker** (`docker`) — weekly updates for base images in `Dockerfile`

Use weekly schedule and sensible defaults (no grouping needed for a repo this size).

### golangci-lint config

The new `.golangci.yml` aligns with library-olm where applicable:

**Linters (final set):**

| Linter | Source | Purpose |
|---|---|---|
| errcheck | library-olm | Unchecked error returns |
| staticcheck | library-olm | Comprehensive static analysis (replaces revive; ST1xxx style checks enabled by default) |
| unused | library-olm | Unused code detection |
| ineffassign | library-olm | Ineffectual assignments |
| misspell | both | Spelling mistakes |
| importas | library-olm | Enforce consistent import aliases |
| errorlint | orb | Error wrapping correctness (errors.Is/As) |
| gocritic | orb | Diagnostic checks |
| unconvert | orb | Unnecessary type conversions |

**Dropped:**
- revive — replaced by staticcheck. The `exported` and `package-comments` checks that revive had disabled are now enabled via staticcheck's `ST1000`/`ST1020`/`ST1021` (on by default).

**Deferred:**
- depguard — orb doesn't have clear import isolation boundaries yet.

**Formatters:**
- gci — enforce import ordering: standard library, third-party, then `prefix(github.com/joelanford/orb)`
- gofmt — already used via `make verify`, now also declared in golangci-lint

**importas aliases** (matching library-olm conventions for shared packages):

| Package | Alias |
|---|---|
| `github.com/blang/semver/v4` | `bsemver` |
| `github.com/Masterminds/semver/v3` | `mmsemver` |

These aliases are already used consistently in orb's codebase; importas will enforce them.

**Exclusions cleanup:**
- Remove the `internal/image/` revive exclusion (revive is dropped; `internal/image/` still exists but the exclusion was revive-specific).
- Keep the `internal/termimage/internal/rasterx` and `oksvg` path exclusions (vendored third-party code).
- Keep the errcheck `.Close` exclusion.
- Keep existing errcheck settings (exclude-functions for fmt.Fprintf, etc.).

### Documentation updates

Update `specs/tech-stack.md`:
- Remove govulncheck from dev dependencies table
- Update golangci-lint linter list to match new config
- Note dependabot in CI/CD section
