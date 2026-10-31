BINARY  := intentions-bench
MODULE  := github.com/nodelogicau/intentions-bench
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X $(MODULE)/internal/impl.Version=$(VERSION)
DIST    := dist

export CGO_ENABLED := 0

.PHONY: all build test lint fmt vet tidy clean check-scenarios

all: build

build:
	go build -trimpath -ldflags '$(LDFLAGS)' -o $(DIST)/$(BINARY) ./cmd/intentions-bench

# Unit tests only: no test calls a model. Tests that need the reference CLI
# run when `intentions` is on PATH and are skipped otherwise.
test:
	go test ./... -count=1

fmt:
	gofmt -l -w .

vet:
	go vet ./...

lint: vet
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run ./... || echo "golangci-lint not installed; ran go vet only"

tidy:
	go mod tidy

# Checks the scenario set against the pinned reference CLI, calling no model.
check-scenarios: build
	$(DIST)/$(BINARY) run --dry-run --impl ref='intentions serve --mcp --workspace {workspace}'

clean:
	rm -rf $(DIST)
