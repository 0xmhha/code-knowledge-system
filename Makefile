GO ?= go
GOLANGCI_LINT ?= golangci-lint

BIN_DIR := bin
LOG_DIR := logs

.PHONY: all build build-bins test test-short race vet lint fmt fmt-check tidy clean help \
        log-clear log-tail

all: build

help:
	@echo "Build:"
	@echo "  build         go build ./..."
	@echo "  build-bins    build cks-mcp, cks-agent, cks-eval into ./bin/"
	@echo ""
	@echo "Quality:"
	@echo "  test          go test -race ./..."
	@echo "  test-short    go test -short ./..."
	@echo "  vet           go vet ./..."
	@echo "  lint          golangci-lint run"
	@echo "  fmt           gofmt -s -w ."
	@echo "  fmt-check     fail if files need gofmt"
	@echo "  tidy          go mod tidy"
	@echo ""
	@echo "Logs (dev cycle: collect -> inspect -> clear -> rerun):"
	@echo "  log-clear     remove ./logs/* (footprint + audit)"
	@echo "  log-tail      show tail of each .jsonl log, jq-prettified if available"
	@echo "                (auditlog.Verify() is exposed as a library; a CLI flag"
	@echo "                 lands with cks-eval in Phase E)"
	@echo ""
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

log-clear:
	@rm -rf $(LOG_DIR)
	@mkdir -p $(LOG_DIR)/footprint $(LOG_DIR)/audit
	@echo "cleared $(LOG_DIR)/{footprint,audit}"

log-tail:
	@found=0; \
	for f in $$(find $(LOG_DIR) -name '*.jsonl' 2>/dev/null); do \
		found=1; \
		echo "===== $$f (last 20) ====="; \
		if command -v jq >/dev/null 2>&1; then \
			tail -n 20 "$$f" | jq -c '.'; \
		else \
			tail -n 20 "$$f"; \
		fi; \
	done; \
	if [ $$found -eq 0 ]; then echo "no logs found in $(LOG_DIR)/"; fi

