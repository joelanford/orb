.DEFAULT_GOAL := build

.PHONY: build
build:
	go build -o orb ./cmd/orb

.PHONY: test
test:
	go test ./... -count=1

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
	git diff --exit-code

.PHONY: release
release:
	go tool goreleaser release --snapshot --clean
