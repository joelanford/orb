# Verification

## Implementation Correctness

- [ ] `make test` passes
- [ ] `make lint` passes
- [ ] `make verify` passes
- [ ] `orb bundle convert plain` produces identical output for a real bundle (spot-check)
- [ ] No `internal/convert` imports outside `internal/convert/` and `internal/helm/`
- [ ] No `internal/bundle` imports for `RegistryV1` type (only `VersionRelease`, `Property`, registry types)
- [ ] `internal/bundle/bundle.go` deleted
- [ ] `internal/bundle/fromfs.go` deleted
- [ ] `internal/convert/certproviders/` deleted
- [ ] `internal/convert/certprovider.go` deleted
- [ ] `internal/convert/generators.go` deleted
- [ ] `internal/convert/resources.go` deleted
- [ ] library-olm version in `go.mod` exports cert provider implementations

## Project Conventions

- [ ] Commit uses `refactor:` prefix per `specs/conventions.md`
- [ ] No exported Go API changes (this is `internal/` only)
- [ ] Design principles from `specs/mission.md` upheld — especially "thin wrapper over library-olm"
- [ ] `specs/tech-stack.md` updated if dependency usage changes
- [ ] `CLAUDE.md` updated if package descriptions change
