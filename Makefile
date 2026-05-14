.DEFAULT_GOAL := build

export GO_BUILD_TAGS := containers_image_openpgp
GO_BUILD_FLAGS := -tags $(GO_BUILD_TAGS)

.PHONY: lint
lint:
	go tool golangci-lint run --build-tags $(GO_BUILD_TAGS) ./...

.PHONY: lint-fix
lint-fix:
	go tool golangci-lint run --build-tags $(GO_BUILD_TAGS) --fix ./...

.PHONY: test
test:
	go test $(GO_BUILD_FLAGS) ./... -race -count=1

.PHONY: build
build:
	go build $(GO_BUILD_FLAGS) -o orb ./cmd/orb

.PHONY: install
install:
	go install $(GO_BUILD_FLAGS) ./cmd/orb

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
