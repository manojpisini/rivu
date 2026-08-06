APP := rivu
VERSION ?= 1.0.0
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
DATE ?= $(shell date -u +%Y-%m-%d)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(DATE)

.PHONY: all tidy fmt test vet build check clean
all: check build

tidy:
	go mod tidy

fmt:
	gofmt -w cmd internal

test:
	go test ./...

vet:
	go vet ./...

build:
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(APP) ./cmd/rivu

check: fmt test vet

clean:
	rm -rf $(APP) dist
