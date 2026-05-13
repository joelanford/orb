# Verification

## Implementation Correctness

- [ ] `make lint` passes with the new golangci-lint config
- [ ] `make test` passes (no behavioral changes)
- [ ] `make verify` passes
- [ ] No `vulncheck` target in Makefile
- [ ] No `vulncheck` job in `.github/workflows/ci.yml`
- [ ] `.github/dependabot.yml` exists with gomod, github-actions, and docker ecosystems
- [ ] `.golangci.yml` enables: errcheck, staticcheck, unused, ineffassign, misspell, importas, errorlint, gocritic, unconvert
- [ ] `.golangci.yml` does not reference revive
- [ ] `.golangci.yml` has gci formatter with `prefix(github.com/joelanford/orb)`
- [ ] `.golangci.yml` has importas configured with `bsemver` and `mmsemver` aliases
- [ ] No revive-specific `internal/image/` exclusion rule remains
- [ ] Import ordering in all `.go` files matches gci expectations (run `go tool golangci-lint run` with no errors)

## Project Conventions

- [ ] Commit uses `chore:` prefix per `specs/conventions.md`
- [ ] No exported Go API changes (this is tooling-only)
- [ ] Design principles from `specs/mission.md` are not violated (client-side CLI, no cluster interaction)
- [ ] `specs/tech-stack.md` accurately reflects the new dev dependency set
- [ ] `CLAUDE.md` build commands are accurate
