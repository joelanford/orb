# orb

Client-side CLI that distills OLM's core features into a standalone tool. Resolves, renders, and inspects operator bundles and catalogs without requiring any on-cluster OLM APIs. Integrates with Helm, kubectl, and OCI registries via the unix philosophy: do one thing well, compose with mature tools.

## Architecture

All logic lives in `internal/`; the only entry point is `cmd/orb/main.go`. No exported Go APIs beyond the CLI binary.

Key packages:
- `internal/cmd/` — cobra command definitions
- `internal/catalog/` — catalog DB (SQLite), FBC parsing, resolution
- `internal/bundle/` — bundle loading and version-release types
- `internal/convert/` — bundle-to-manifest conversion
- `internal/destination/` — output targets (plain manifests, Helm charts)
- `internal/helm/` — Helm chart generation and packaging
- `internal/source/` — registry+v1 bundle source loading
- `internal/image/` — OCI image handling
- `internal/transport/` — transport prefix parsing (docker://, oci:, dir:, etc.)

## Design Principles

1. Client-side first — no cluster-side components
2. Leverage existing ecosystems — integrate with Helm, kubectl, OCI registries
3. OLM content compatibility — faithfully consume/produce OLM ecosystem artifacts
4. Progressive disclosure — simple one-liners for common tasks
5. Unix philosophy — compose with other tools, no direct cluster interaction

## Build, Test, Run

```sh
make build       # go build -o orb ./cmd/orb
make test        # go test ./... -race -count=1
make lint        # go tool golangci-lint run
make verify      # go mod tidy + gofmt -s + go vet + hack/diff.sh
make vulncheck   # go tool govulncheck ./...
make release     # goreleaser (snapshot by default)
```

All PRs must pass `make test`, `make lint`, and `make verify`.

## Conventions

- **Commits:** conventional commits (`feat:`, `fix:`, `refactor:`, `chore:`, `docs:`, `test:`, `perf:`). Imperative, lowercase after prefix, no period.
- **Branches:** free-form slugs (e.g., `catalog-search`, `fix-yaml-quoting`)
- **PRs:** use the existing template (Description, Changes, Testing, Checklist)
- See `specs/conventions.md` for full details and examples.

## SDD Workflow

Work items are tracked as spec directories under `specs/YYYY-MM-DD-<slug>/` with status frontmatter (`idea`, `ready`, `in-progress`, `pr-submitted`, `done`).

| Command | Purpose |
|---|---|
| `/sdd-plan-next-phase` | Pick the next work item, create/refine its spec |
| `/sdd-implement` | Implement a work item from its spec |
| `/sdd-review` | Review branch changes against specs |
| `/sdd-ship` | Verify, commit, and publish |
| `/sdd-ideate` | Brainstorm and add new work items |
| `/sdd-quick-item` | Quickly capture a work item idea |

Governing specs: `specs/mission.md`, `specs/tech-stack.md`, `specs/conventions.md`.
