BINARY  := bin/jk
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build test vet fmt fmt-check lint-lite cross tidy clean docs-gen docs-check docs docs-serve release-check snapshot

all: lint-lite test build

build:
	CGO_ENABLED=0 go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/jk

test:
	go test -count=1 ./...

vet:
	go vet ./...

fmt:
	gofmt -w .

fmt-check:
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

# Lightweight lint without golangci-lint: formatting, vet, tidy go.mod and the
# rule that internal/kit must not import Jenkins-specific packages.
lint-lite: fmt-check vet
	go mod tidy -diff
	@if go list -deps ./internal/kit/... | grep -E 'jk/internal/(core|client|cli|mcpserver)'; then \
		echo "internal/kit must not import core, client, cli or mcpserver"; exit 1; fi

cross:
	CGO_ENABLED=0 GOOS=linux   GOARCH=amd64 go build ./...
	CGO_ENABLED=0 GOOS=linux   GOARCH=arm64 go build ./...
	CGO_ENABLED=0 GOOS=darwin  GOARCH=arm64 go build ./...
	CGO_ENABLED=0 GOOS=darwin  GOARCH=amd64 go build ./...
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...

tidy:
	go mod tidy

# Docs: the CLI reference in docs/reference is generated from the cobra tree.
docs-gen:
	go run ./cmd/gendocs docs/reference

# Fails when docs/reference is out of date with the CLI (used by CI).
docs-check: docs-gen
	@git diff --exit-code -- docs/reference || { echo "docs/reference is out of date: run 'make docs-gen' and commit"; exit 1; }
	@if [ -n "$$(git ls-files --others --exclude-standard docs/reference)" ]; then \
		echo "new files in docs/reference: run 'make docs-gen' and commit"; exit 1; fi

# Needs: pip install -r docs/requirements.txt
docs: docs-gen
	mkdocs build --strict

docs-serve: docs-gen
	mkdocs serve

# Release tooling (needs goreleaser v2: brew install goreleaser).
release-check:
	goreleaser check

snapshot:
	goreleaser release --snapshot --clean

clean:
	rm -rf bin dist site
