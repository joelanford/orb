# Requirements

- govulncheck is removed from CI and Makefile entirely
- Dependabot is configured for Go modules, GitHub Actions, and Docker ecosystems
- golangci-lint config enables: errcheck, staticcheck, unused, ineffassign, misspell, importas, errorlint, gocritic, unconvert
- golangci-lint config drops revive
- gci formatter enforces import ordering with `prefix(github.com/joelanford/orb)`
- importas enforces `bsemver` and `mmsemver` aliases
- All existing code passes the new lint config (fix violations if any)
- `specs/tech-stack.md` reflects the updated tooling

## Acceptance Criteria

- `make lint` passes with the new config
- `make test` still passes (no behavioral changes)
- `make verify` still passes
- CI workflow no longer has a `vulncheck` job
- `.github/dependabot.yml` exists and is valid
- No revive references remain in `.golangci.yml`
- Import ordering in all `.go` files matches gci's expected format
