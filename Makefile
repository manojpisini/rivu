APP := rivu
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: all tidy tidy-check fmt fmt-check lint test vet cover check build install snapshot dist clean
all: check build

tidy:
	go mod tidy

# Fails when go.mod/go.sum would change (CI gate).
tidy-check:
	go mod tidy
	git diff --exit-code go.mod go.sum

fmt:
	gofmt -w cmd internal

# Fails when gofmt would rewrite anything (prints the files first),
# and never passes vacuously when gofmt itself is missing.
fmt-check:
	@command -v gofmt >/dev/null 2>&1 || { echo "fmt-check: gofmt not on PATH"; exit 1; }
	@out=$$(gofmt -l cmd internal); if [ -n "$$out" ]; then echo "$$out"; echo "gofmt: run 'make fmt'"; exit 1; fi

lint:
	golangci-lint run ./...

test:
	go test ./...

vet:
	go vet ./...

cover:
	go test ./... "-coverprofile=cover.out"
	go tool cover "-func=cover.out"

check: fmt-check test vet

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(APP) ./cmd/rivu

install:
	go install -trimpath -ldflags "$(LDFLAGS)" ./cmd/rivu

# Full goreleaser snapshot: 6 targets, archives, checksums, SBOMs.
snapshot:
	go run github.com/goreleaser/goreleaser/v2@latest release --snapshot --clean

dist: snapshot

clean:
	rm -rf $(APP) dist cover.out
