# Implementation Plan

1. **Remove govulncheck**
   - Delete the `vulncheck` target from `Makefile`
   - Delete the `vulncheck` job from `.github/workflows/ci.yml`
   - Remove `govulncheck` tool dependency from `go.mod` if present (run `go mod tidy`)

2. **Add dependabot configuration**
   - Create `.github/dependabot.yml` with three ecosystem entries:
     - `gomod` (directory: `/`, schedule: weekly)
     - `github-actions` (directory: `/`, schedule: weekly)
     - `docker` (directory: `/`, schedule: weekly)

3. **Update golangci-lint config**
   - Replace the linters block: drop revive, add staticcheck, unused, ineffassign, errcheck, importas
   - Add formatters block with gci (sections: standard, default, `prefix(github.com/joelanford/orb)`) and gofmt
   - Add importas settings with `bsemver` and `mmsemver` aliases, `no-unaliased: true`
   - Remove revive settings section
   - Remove the `internal/image/` revive exclusion rule (dead path)
   - Keep errcheck settings, errorlint settings, gocritic settings, path exclusions, and errcheck Close exclusion

4. **Fix lint violations**
   - Run `make lint` and fix any new violations from staticcheck, unused, ineffassign, errcheck, or importas
   - Run `go tool golangci-lint run --fix` to auto-fix gci import ordering if needed
   - Verify `make lint` passes cleanly

5. **Update documentation**
   - Update `specs/tech-stack.md`: remove govulncheck row, update golangci-lint linter list, add dependabot note to CI/CD section
   - Update `CLAUDE.md` build commands table if it references vulncheck

6. **Final verification**
   - Run `make test`, `make lint`, `make verify` to confirm everything passes
