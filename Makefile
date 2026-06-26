GO ?= go
GOLANGCI_LINT ?= golangci-lint

BIN_DIR := bin
LOG_DIR := logs

# cks-mcp build stamp — injected into main.builderVersion and surfaced via
# cks_ops_health.builder_version, so a running MCP can be matched to the source
# commit it was built from ("-dirty" flags an uncommitted build).
CKS_COMMIT  := $(shell git describe --always --dirty --abbrev=8 2>/dev/null || echo unknown)
CKS_VERSION := cks-mcp/0.1.0-$(CKS_COMMIT)
CKS_LDFLAGS := -X main.builderVersion=$(CKS_VERSION)

# Dogfood eval (cks indexing cks itself) — used to produce the retrieval
# baseline in eval/reports/. Override these to point at a different
# target repo for cross-repo eval runs.
CKG            ?= ckg
CKG_DATA       ?= .ckg-data
CKG_SRC        ?= .
CKG_LANG       ?= go
CKV            ?= ckv
CKV_DATA       ?= .ckv-data
CKV_SRC        ?= .
CKV_EMBEDDER   ?= mock
# USE_CKV controls whether dogfood-eval wires ckv as a real backend.
# Default 0 because:
#   - ckv with --embedder=mock gives no semantic uplift over the in-memory
#     Fake (hash-based embeddings, not semantic), but doubles latency
#     (~120ms -> ~250ms per scenario) and occasionally trips a
#     "transport closed" error in the cks-mcp -> ckv subprocess link.
#     Opt-in keeps the default fast and stable; flip to USE_CKV=1 once
#     bgeonnx is configured and task #39 (subprocess restart) lands.
USE_CKV        ?= 0
DOGFOOD_CONFIG ?= cks-dogfood.yaml
EVAL_SCENARIOS ?= eval/scenarios
EVAL_REPORT    ?= eval/reports/baseline-dogfood.json

.PHONY: all build build-bins test test-short race vet lint fmt fmt-check tidy clean help \
        log-clear log-tail \
        ckg-index ckg-index-clean ckv-index ckv-index-clean \
        dogfood-config dogfood-eval dogfood-eval-summary

all: build

help:
	@echo "Build:"
	@echo "  build         go build ./..."
	@echo "  build-bins    build cks-mcp, cks-agent, cks-eval, cks-glossary-gen,"
	@echo "                cks-inventory-check, cks-entry-verify, cks-anchor-refresh into ./bin/"
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
	@echo "Dogfood eval (cks indexing cks itself):"
	@echo "  ckg-index           build a fresh ckg index from \$$(CKG_SRC) into \$$(CKG_DATA)"
	@echo "  ckg-index-clean     remove \$$(CKG_DATA)"
	@echo "  ckv-index           build a fresh ckv index from \$$(CKV_SRC) into \$$(CKV_DATA)"
	@echo "  ckv-index-clean     remove \$$(CKV_DATA)"
	@echo "  dogfood-config      generate \$$(DOGFOOD_CONFIG) (cks.yaml; ckv block iff USE_CKV=1)"
	@echo "  dogfood-eval        ckg-only pipeline (default — stable, ~10s)"
	@echo "  dogfood-eval        USE_CKV=1 pipeline (real ckv subprocess — ~2 min, less stable)"
	@echo "  dogfood-eval-summary  print one-line metric summary from \$$(EVAL_REPORT)"
	@echo "                  overrides: CKG=/path/to/ckg, CKV=/path/to/ckv, CKV_EMBEDDER, ..."
	@echo ""
	@echo "  clean         remove ./bin and go build cache"

build:
	$(GO) build ./...

build-bins:
	@mkdir -p $(BIN_DIR)
	$(GO) build -ldflags "$(CKS_LDFLAGS)" -o $(BIN_DIR)/cks-mcp ./cmd/cks-mcp
	$(GO) build -o $(BIN_DIR)/cks-agent           ./cmd/cks-agent
	$(GO) build -o $(BIN_DIR)/cks-eval            ./cmd/cks-eval
	$(GO) build -o $(BIN_DIR)/cks-glossary-gen    ./cmd/cks-glossary-gen
	$(GO) build -o $(BIN_DIR)/cks-inventory-check ./cmd/cks-inventory-check
	$(GO) build -o $(BIN_DIR)/cks-entry-verify    ./cmd/cks-entry-verify
	$(GO) build -o $(BIN_DIR)/cks-anchor-refresh  ./cmd/cks-anchor-refresh

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

clean: ckg-index-clean ckv-index-clean
	rm -rf $(BIN_DIR) $(DOGFOOD_CONFIG)
	@echo "cleaned $(BIN_DIR), $(CKG_DATA), $(CKV_DATA), $(DOGFOOD_CONFIG)"
	@echo "(go build cache untouched — run 'go clean -cache' manually)"

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

# ---------------------------------------------------------------------
# Dogfood eval — cks-mcp runs against a ckg index built from cks itself,
# then cks-eval scores the corpus and writes a JSON report.
#
# The whole flow is wire-up only — every artifact is rebuilt every time
# so the resulting baseline always reflects the current working tree.
# Override CKG / CKG_SRC / CKG_DATA / DOGFOOD_CONFIG / EVAL_REPORT to
# retarget against a different repo or a different scenario set.
# ---------------------------------------------------------------------

# ckg-index always rebuilds (.PHONY): cks changes invalidate the index,
# and ckg's own incremental cache is fast enough that a clean rebuild
# is cheap.
ckg-index:
	@command -v $(CKG) >/dev/null 2>&1 || { \
		echo "ckg binary not found on PATH; set CKG=/path/to/ckg"; exit 1; }
	@rm -rf $(CKG_DATA)
	@mkdir -p $(CKG_DATA)
	$(CKG) build --src $(CKG_SRC) --out $(CKG_DATA) --lang $(CKG_LANG)
	@echo "ckg index built: $(CKG_DATA)/graph.db"

ckg-index-clean:
	rm -rf $(CKG_DATA)

ckv-index:
	@command -v $(CKV) >/dev/null 2>&1 || { \
		echo "ckv binary not found on PATH; set CKV=/path/to/ckv"; exit 1; }
	@rm -rf $(CKV_DATA)
	@mkdir -p $(CKV_DATA)
	$(CKV) --embedder=$(CKV_EMBEDDER) build --src $(CKV_SRC) --out $(CKV_DATA)
	@echo "ckv index built: $(CKV_DATA)/vector.db"

ckv-index-clean:
	rm -rf $(CKV_DATA)

# dogfood-config writes a minimal cks.yaml that points at the local
# indexes. Skips overwrite if the file already exists; delete it first
# to regenerate. The ckv block fires only when USE_CKV=1 (default 0)
# because the ckv subprocess link is currently unstable (see USE_CKV
# doc above). With USE_CKV=0 the cks-mcp ckv adapter stays on the
# in-memory Fake.
dogfood-config:
	@if [ -f $(DOGFOOD_CONFIG) ]; then \
		echo "$(DOGFOOD_CONFIG) already exists (delete to regenerate)"; \
	else \
		ckv_path=""; ckv_bin=""; \
		if [ "$(USE_CKV)" = "1" ]; then \
			ckv_path="./$(CKV_DATA)"; \
			ckv_bin=$$(command -v $(CKV) || echo $(CKV)); \
		fi; \
		printf '%s\n' \
			'version: 1' \
			'backends:' \
			'  ckg:' \
			'    path: ./$(CKG_DATA)/graph.db' \
			'    timeout_ms: 5000' \
			'  ckv:' \
			"    path: \"$$ckv_path\"" \
			"    binary_path: \"$$ckv_bin\"" \
			"    embed_model: $(CKV_EMBEDDER)" \
			'    timeout_ms: 3000' \
			'listen:' \
			'  http_addr: "127.0.0.1:8080"' \
			'  mcp_stdio: true' \
			'logging:' \
			'  level: info' \
			'  mode: prod' \
			'  footprint_dir: "./logs/footprint"' \
			'  audit_dir: "./logs/audit"' \
			'sanitize:' \
			'  rules_path: "./policies/sanitization_rules.yaml"' \
			'  default_action: "drop"' \
			'  fail_closed_on_unknown_rule: true' \
			> $(DOGFOOD_CONFIG); \
		echo "wrote $(DOGFOOD_CONFIG) (USE_CKV=$(USE_CKV))"; \
	fi

# dogfood-eval-deps switches the ckv-index prerequisite on USE_CKV.
DOGFOOD_DEPS = build-bins ckg-index dogfood-config
ifeq ($(USE_CKV),1)
DOGFOOD_DEPS += ckv-index
endif

dogfood-eval: $(DOGFOOD_DEPS)
	@mkdir -p $$(dirname $(EVAL_REPORT))
	./$(BIN_DIR)/cks-eval \
		-scenarios $(EVAL_SCENARIOS) \
		-cks-mcp ./$(BIN_DIR)/cks-mcp \
		-config $(DOGFOOD_CONFIG) \
		-output $(EVAL_REPORT)
	@echo ""
	@$(MAKE) -s dogfood-eval-summary

dogfood-eval-summary:
	@if [ ! -f $(EVAL_REPORT) ]; then \
		echo "no report at $(EVAL_REPORT) — run 'make dogfood-eval' first"; \
		exit 1; \
	fi
	@python3 -c "import json; \
		d = json.load(open('$(EVAL_REPORT)')); \
		print('='*70); \
		print('cks-eval baseline (%d scenarios, schema v%d)' % (len(d['results']), d['schema_version'])); \
		print('='*70); \
		print('%-34s %-22s %5s %5s %5s' % ('scenario', 'intent', 'P', 'R', 'F1')); \
		print('-'*70); \
		[print('%-34s %-22s %5.2f %5.2f %5.2f' % (r['name'], r.get('intent','-'), r['metrics']['file_precision'], r['metrics']['file_recall'], r['metrics']['file_f1'])) for r in d['results']]; \
		ok = [r for r in d['results'] if not r.get('error')]; \
		avg_r = sum(r['metrics']['file_recall'] for r in ok)/len(ok) if ok else 0; \
		print('-'*70); \
		print('avg recall (errored runs excluded): %.3f over %d scenarios' % (avg_r, len(ok))); \
		print(); \
		print('per-intent rollup:'); \
		[print('  %-22s n=%d R=%.2f lat=%dms' % (s['intent'], s['count'], s['avg_recall'], s['avg_latency_ms'])) for s in d.get('intent_summary', [])]"


