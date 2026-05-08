# Tech Stack

## Language & Runtime

- **Go 1.25** (single module: `github.com/joelanford/orb`)
- **CGO disabled** for release builds

## CLI Framework

- **cobra** for command structure and flag parsing

## Core Dependencies

| Dependency | Purpose |
|---|---|
| operator-framework/api | OLM API types (bundles, packages, channels) |
| operator-framework/operator-registry | FBC (file-based catalog) parsing and declcfg types |
| joelanford/library-olm | OCI image handling, caching, bundle/catalog handlers, plain manifest conversion, validation, cert providers, catalog store (catalog/v1), FBC import with extension hooks (catalog/fbc), bundle resolution (resolver/v1) |
| containers/image (go.podman.io/image/v5) | OCI image transport and registry interaction |
| helm.sh/helm/v4 | Helm chart generation, packaging, and plugin SDK |
| Masterminds/semver/v3, blang/semver/v4 | Semver parsing and constraint matching |
| lipgloss/v2, termenv | Terminal styling and output formatting |
| spf13/cobra | CLI framework |

## Dev Dependencies

| Tool | Purpose |
|---|---|
| testify | Test assertions and helpers |
| golangci-lint | Linting (errcheck, errorlint, gocritic, importas, ineffassign, misspell, staticcheck, unconvert, unused); formatting (gci, gofmt) |
| goreleaser v2 | Cross-platform builds, container images, and releases |

## Project Structure

```
cmd/orb/main.go          # Entry point
internal/
  cmd/                   # Cobra command definitions
  catalog/               # Catalog store (library-olm wrapper), FBC extension for display metadata
  convert/               # Naming and hashing utilities (used by helm)
  destination/           # Output targets (plain manifests, Helm charts)
  helm/                  # Helm chart generation and packaging
  progress/              # Download progress tracking (wraps library-olm image.Repository)
  source/                # Registry+v1 bundle source loading
  termimage/             # Terminal image rendering (SVG/raster)
  transport/             # Transport prefix parsing (docker://, oci:, dir:, etc.)
helm-plugins/            # Helm plugin definitions (orb-getter)
hack/                    # Helper scripts (diff.sh)
assets/                  # Logo and static assets
specs/                   # Governing specs and work item specs
.claude/commands/        # SDD workflow commands
```

## Build Commands

| Command | Description |
|---|---|
| `make build` | Build the binary (`go build -o orb ./cmd/orb`) |
| `make install` | Install to GOPATH (`go install ./cmd/orb`) |
| `make test` | Run tests with race detector (`go test ./... -race -count=1`) |
| `make lint` | Run golangci-lint (`go tool golangci-lint run`) |
| `make lint-fix` | Run golangci-lint with auto-fix (`go tool golangci-lint run --fix`) |
| `make verify` | Run tidy and lint-fix, then check for uncommitted changes |
| `make release` | Build release artifacts via goreleaser |

## Containerization

- **Dockerfile:** distroless static base image, single binary copy
- **goreleaser:** builds multi-platform images (linux/amd64, linux/arm64) and pushes to `ghcr.io/joelanford/orb`

## CI/CD

- **GitHub Actions:** `ci.yml` (test/lint/verify on PRs) and `release.yml` (goreleaser on tags)
- **Dependabot:** automated dependency updates for Go modules, GitHub Actions, and Docker