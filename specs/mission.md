# Mission

orb distills OLM's core features into a client-side CLI. Users get a full-featured experience from OLM's content ecosystem without requiring any on-cluster OLM APIs.

## Goals

1. Provide a fast, client-side CLI for resolving and inspecting operator bundles and catalogs
2. Convert OLM bundle formats to plain Kubernetes manifests or Helm charts
3. Integrate with Helm via plugins (orb-getter) for seamless catalog-to-cluster workflows
4. Support multiple OCI image transports (docker://, oci:, dir:, tar:, etc.)
5. Keep the tool self-contained with no cluster-side dependencies
6. Support one-shot imperative operations like health checks

## Non-Goals

- Not a registry or server — purely client-side
- No direct interaction with Kubernetes clusters; defer to kubectl, Helm, ArgoCD, etc.
- No runtime operator lifecycle management (continuous reconciliation, upgrade controllers)

## Design Principles

1. **Client-side first** — no cluster-side components; the CLI is self-contained
2. **Leverage existing ecosystems** — integrate with Helm, kubectl, and OCI registries rather than inventing new workflows
3. **OLM content compatibility** — faithfully consume and produce artifacts from OLM's content ecosystem
4. **Progressive disclosure** — simple one-liners for common tasks, deeper options for advanced use
5. **Unix philosophy** — do one thing well, compose with other mature tools
6. **Thin wrapper over library-olm** — core OLM functionality (resolution, FBC parsing, bundle handling) should be imported from library-olm; building core OLM logic directly in this repo is an anti-pattern

## Development Practices

- All PRs must pass `make test`, `make lint`, and `make verify`
- All logic lives in `internal/`; the only entry point is `cmd/orb/main.go`
- No exported Go APIs beyond the CLI binary itself
