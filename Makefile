build:
	go build -o bin/kernel ./cmd/kernel

test:
	go vet ./...
	go test ./...

lint:
	@golangci-lint run || true

changelog:
	@chglog --next-tag $(shell git describe --tags --abbrev=0)

clean-templates:
	find pkg/templates -type d -name "node_modules" -exec rm -rf {} + 2>/dev/null || true
	find pkg/templates -type d -name ".venv" -exec rm -rf {} + 2>/dev/null || true

release-dry-run: clean-templates
	goreleaser check
	goreleaser healthcheck
	goreleaser release --snapshot --clean

release: clean-templates
	goreleaser release --clean

.PHONY: build test lint run changelog release clean-templates
