BINARY  := bin/jk
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: all build test vet fmt fmt-check lint-lite cross tidy clean

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

clean:
	rm -rf bin dist
