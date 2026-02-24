.DEFAULT_GOAL := build

.PHONY: build
build:
	go build -o orb ./cmd/orb

.PHONY: install
install:
	go install ./cmd/orb

.PHONY: test
test:
	go test ./... -race -count=1

.PHONY: lint
lint:
	go tool golangci-lint run

.PHONY: vulncheck
vulncheck:
	go tool govulncheck ./...

.PHONY: verify
verify:
	go mod tidy
	gofmt -w -s .
	go vet ./...
	./hack/diff.sh

export IMAGE_TAG ?= dev
export ENABLE_RELEASE_PIPELINE ?= false
GORELEASER_ARGS ?= --snapshot --clean

.PHONY: release
release:
	@docker buildx use goreleaser 2>/dev/null || docker buildx create --name goreleaser --use
	go tool goreleaser release $(GORELEASER_ARGS)
