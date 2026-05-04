.DEFAULT_GOAL := build

.PHONY: lint
lint:
	go tool golangci-lint run ./...

.PHONY: lint-fix
lint-fix:
	go tool golangci-lint run --fix ./...

.PHONY: test
test:
	go test ./... -race -count=1

.PHONY: build
build:
	go build -o orb ./cmd/orb

.PHONY: install
install:
	go install ./cmd/orb

.PHONY: verify
verify:
	./hack/diff.sh tidy lint-fix

.PHONY: tidy
tidy:
	go mod tidy

export IMAGE_TAG ?= dev
export ENABLE_RELEASE_PIPELINE ?= false
GORELEASER_ARGS ?= --snapshot --clean

.PHONY: release
release:
	@docker buildx use goreleaser 2>/dev/null || docker buildx create --name goreleaser --use
	go tool goreleaser release $(GORELEASER_ARGS)
