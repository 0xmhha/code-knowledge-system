GO ?= go
GOLANGCI_LINT ?= golangci-lint

BIN_DIR := bin

.PHONY: all build build-bins test test-short race vet lint fmt fmt-check tidy clean help

all: build

help:
	@echo "Targets:"
	@echo "  build         go build ./..."
	@echo "  build-bins    build cks-mcp, cks-agent, cks-eval into ./bin/"
	@echo "  test          go test -race ./..."
	@echo "  test-short    go test -short ./..."
	@echo "  vet           go vet ./..."
	@echo "  lint          golangci-lint run"
	@echo "  fmt           gofmt -s -w ."
	@echo "  fmt-check     fail if files need gofmt"
	@echo "  tidy          go mod tidy"
	@echo "  clean         remove ./bin and go build cache"

build:
	$(GO) build ./...

build-bins:
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/cks-mcp   ./cmd/cks-mcp
	$(GO) build -o $(BIN_DIR)/cks-agent ./cmd/cks-agent
	$(GO) build -o $(BIN_DIR)/cks-eval  ./cmd/cks-eval

test:
	$(GO) test -race ./...

test-short:
	$(GO) test -short ./...

race: test

vet:
	$(GO) vet ./...

lint:
	$(GOLANGCI_LINT) run

fmt:
	gofmt -s -w .

fmt-check:
	@diff=$$(gofmt -s -l .); \
	if [ -n "$$diff" ]; then \
		echo "gofmt needs to format:"; echo "$$diff"; exit 1; \
	fi

tidy:
	$(GO) mod tidy

clean:
	rm -rf $(BIN_DIR)
	$(GO) clean -cache
