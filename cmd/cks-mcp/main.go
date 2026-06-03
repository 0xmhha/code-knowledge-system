// Command cks-mcp serves the cks composer pipeline over an MCP (Model
// Context Protocol) stdio transport.
//
// Phase C.5 (slim) registers two tools: cks.context.get_for_task and
// cks.ops.health. See internal/mcp for handler details.
//
// Backend wiring (post R1′ G1/G2):
//   - ckg: if config.Backends.CKG.Path is set, the binary opens a real
//     ckg SQLite store via ckgclient.NewReal. When empty, it falls back
//     to the Smart Dummy.
//   - ckv: if config.Backends.CKV.Path is set, the binary opens the index
//     in-process via ckvclient.NewReal (pkg/ckv + an Ollama bge-m3 embedder
//     constructed here and shared with the intent classifier) — no
//     subprocess. When the path is empty, or Ollama is unreachable, it
//     falls back to the Smart Dummy and cks.ops.health reports "degraded".
//
// The swap surface is intentionally a constructor choice in this file,
// not a composer change: the composer depends on the ckg/ckv Client
// interfaces, not on their implementations.
//
// Usage:
//
//	cks-mcp -config ./policies/cks.yaml.example
//
// If -config is omitted, config.Default() supplies sane defaults; the
// sanitize ruleset is still loaded from disk when SanitizeConfig.RulesPath
// is set (the policies/sanitization_rules.yaml baseline is required for
// any non-trivial use).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/0xmhha/code-knowledge-vector/pkg/embed/ollama"
	ckvtypes "github.com/0xmhha/code-knowledge-vector/pkg/types"

	"github.com/0xmhha/code-knowledge-system/internal/ckgclient"
	"github.com/0xmhha/code-knowledge-system/internal/ckvclient"
	"github.com/0xmhha/code-knowledge-system/internal/composer"
	"github.com/0xmhha/code-knowledge-system/internal/composer/budget"
	"github.com/0xmhha/code-knowledge-system/internal/composer/intent"
	"github.com/0xmhha/code-knowledge-system/internal/composer/sanitize"
	"github.com/0xmhha/code-knowledge-system/internal/composer/stage1"
	"github.com/0xmhha/code-knowledge-system/internal/composer/stage2"
	"github.com/0xmhha/code-knowledge-system/internal/composer/stage3"
	"github.com/0xmhha/code-knowledge-system/internal/config"
	"github.com/0xmhha/code-knowledge-system/internal/footprint"
	cksmcp "github.com/0xmhha/code-knowledge-system/internal/mcp"
	"github.com/0xmhha/code-knowledge-system/internal/vocab"
)

// builderVersion is stamped into the MCP server name/version handshake and
// into cks.ops.health responses. Override at build time:
//
//	go build -ldflags "-X main.builderVersion=cks-mcp/0.1.0-$(git rev-parse --short HEAD)"
var builderVersion = "cks-mcp/0.0.1-dev"

func main() {
	configPath := flag.String("config", "", "path to cks.yaml (optional; falls back to defaults)")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx, *configPath); err != nil {
		log.Printf("cks-mcp: %v", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, configPath string) error {
	cfg, err := loadConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	if !cfg.Listen.MCPStdio {
		return errors.New("config.listen.mcp_stdio is false; cks-mcp serves stdio only")
	}

	ruleset, err := loadRuleset(cfg.Sanitize.RulesPath)
	if err != nil {
		return fmt.Errorf("load sanitize ruleset: %w", err)
	}

	ckg, ckgCloser, err := buildCKGClient(cfg.Backends.CKG.Path)
	if err != nil {
		return fmt.Errorf("build ckg client: %w", err)
	}
	defer func() { _ = ckgCloser() }()

	// Construct the shared bge-m3 embedder ONCE (G2). The ckv adapter and the
	// intent classifier use the SAME instance so anchor, query, and chunk
	// vectors live in one model space. On Ollama failure we enter degraded
	// mode (S5): a Smart Dummy ckv + a 1024-dim fake intent embedder, with
	// cks.ops.health reporting "degraded" — never a crash.
	var (
		ckvEmb         ckvtypes.Embedder
		degradedReason string
	)
	if cfg.Backends.CKV.Path != "" {
		emb, embErr := buildEmbedder(cfg.Backends.CKV)
		if embErr != nil {
			degradedReason = embErr.Error()
			log.Printf("cks-mcp: ckv embedder unavailable (%v) — degraded mode", embErr)
		} else {
			ckvEmb = emb
		}
	}

	ckv, ckvCloser, err := buildCKVClient(ctx, cfg.Backends.CKV, ckvEmb, degradedReason)
	if err != nil {
		return fmt.Errorf("build ckv client: %w", err)
	}
	defer func() { _ = ckvCloser() }()

	fp, fpCloser, err := buildFootprint(cfg.Logging)
	if err != nil {
		return fmt.Errorf("build footprint: %w", err)
	}
	defer func() { _ = fpCloser() }()

	// Intent embedder: the shared bge-m3 adapter (bridged to intent's
	// single-text Embedder) when available; otherwise a 1024-dim fake so the
	// pipeline still runs (intent becomes noise → IntentUnknown fan-out,
	// acceptable in degraded mode). Dim 1024 (not the old 32) keeps any
	// downstream dim assumptions consistent with the real model.
	var embedder intent.Embedder
	if ckvEmb != nil {
		embedder = intentEmbedderAdapter{e: ckvEmb}
	} else {
		embedder = &intent.FakeEmbedder{Dim: 1024}
	}
	// Real BodyFetcher: read the cited line range from disk. The
	// indexed source tree at cfg.Backends.CKG.SourceRoot (cwd when
	// empty) MUST match the snapshot ckg was built against — otherwise
	// the body returned for a citation could differ from what was
	// indexed. cks does not enforce this; Citation.CommitHash on the
	// returned EvidencePack is the operator's drift signal.
	fetcher := &budget.FilesystemFetcher{Root: cfg.Backends.CKG.SourceRoot}

	vocabResolver, err := buildVocabResolver(cfg.Vocab.GlossaryPath)
	if err != nil {
		return fmt.Errorf("vocab.Load: %w", err)
	}

	c, err := buildComposer(ctx, ckg, ckv, embedder, fetcher, ruleset, vocabResolver, fp)
	if err != nil {
		return fmt.Errorf("build composer: %w", err)
	}

	deps := cksmcp.Deps{
		Composer:       c,
		CKG:            ckg,
		CKV:            ckv,
		BuilderVersion: builderVersion,
		Index: cksmcp.IndexConfig{
			CKVBinary:   cfg.Backends.CKV.BinaryPath,
			CKGBinary:   cfg.Backends.CKG.BinaryPath,
			CKVDataPath: cfg.Backends.CKV.Path,
			CKGDataPath: cfg.Backends.CKG.Path,
			SourceRoot:  cfg.Backends.CKG.SourceRoot,
			EmbedModel:  cfg.Backends.CKV.EmbedModel,
			OllamaURL:   cfg.Backends.CKV.OllamaURL,
		},
	}
	return cksmcp.Run(ctx, deps)
}

// buildCKGClient picks the real adapter when a path is configured and
// falls back to the Smart Dummy otherwise. The Dummy records each
// would-have-been ckg call on the Composer's instruction collector so
// the upstream LLM can fulfil the request via skills against the
// go-stablenet source tree. The returned closer should be deferred by
// the caller; the Dummy's closer is a no-op.
func buildCKGClient(path string) (ckgclient.Client, func() error, error) {
	if path == "" {
		d := ckgclient.NewDummy()
		return d, d.Close, nil
	}
	real, err := ckgclient.NewReal(path)
	if err != nil {
		return nil, func() error { return nil }, err
	}
	return real, real.Close, nil
}

// buildVocabResolver loads the project glossary at the configured path
// and wraps it in a vocab.Resolver. An empty path skips loading and
// returns (nil, nil) — Stage 1 treats a nil resolver as "no expansion"
// and continues with the verbatim prompt, so vocab is strictly opt-in.
// A path that points at a missing or malformed file is fatal here; we
// would rather refuse to start than silently lose the expansion the
// operator asked for.
func buildVocabResolver(path string) (*vocab.Resolver, error) {
	if path == "" {
		return nil, nil
	}
	return vocab.Load(path)
}

// buildCKVClient picks the in-process ckv adapter (G1: pkg/ckv imported
// directly, no subprocess) when a data path AND a working embedder are
// available, and falls back to the Smart Dummy otherwise. The fallback never
// errors — a configured-but-unavailable ckv degrades (Smart Dummy +
// degraded health, S5) rather than crashing the server. The Dummy records
// each would-have-been ckv call on the Composer's instruction collector so
// the upstream LLM can fulfil the request via skills against go-stablenet.
func buildCKVClient(ctx context.Context, cfg config.CKVConfig, emb ckvtypes.Embedder, degradedReason string) (ckvclient.Client, func() error, error) {
	if cfg.Path == "" {
		d := ckvclient.NewDummy()
		return d, d.Close, nil
	}
	if emb == nil {
		// Configured but the embedder couldn't be built (Ollama down).
		d := ckvclient.NewDegradedDummy(degradedReason)
		return d, d.Close, nil
	}
	real, err := ckvclient.NewReal(ctx, ckvclient.RealOpts{DataPath: cfg.Path, Embedder: emb})
	if err != nil {
		// ckv.Open failed (index identity mismatch, missing files): degrade
		// instead of crashing, and surface why via health (S5).
		log.Printf("cks-mcp: ckv.Open failed (%v) — degraded mode", err)
		d := ckvclient.NewDegradedDummy(err.Error())
		return d, d.Close, nil
	}
	return real, real.Close, nil
}

// buildEmbedder constructs the shared Ollama embedder (a ckv types.Embedder)
// for the configured model. For bge-m3 it asserts the probed dimension is
// 1024 (S3): the only failure mode the manifest identity check can't catch is
// "Ollama served a different-dimensioned model under the bge-m3 name."
func buildEmbedder(cfg config.CKVConfig) (ckvtypes.Embedder, error) {
	model := cfg.EmbedModel
	if model == "" {
		model = "bge-m3"
	}
	adapter, err := ollama.Open(ollama.Options{Endpoint: cfg.OllamaURL, ModelName: model})
	if err != nil {
		return nil, err
	}
	if model == "bge-m3" {
		if got := adapter.Dimension(); got != 1024 {
			_ = adapter.Close()
			return nil, fmt.Errorf("embedder %q dim=%d, want 1024 (bge-m3)", model, got)
		}
	}
	return adapter, nil
}

// intentEmbedderAdapter bridges a ckv types.Embedder (batch API) to the
// intent package's single-text Embedder, so one Ollama adapter serves both
// the ckv chunk space and the intent anchor space (G2).
type intentEmbedderAdapter struct{ e ckvtypes.Embedder }

func (a intentEmbedderAdapter) Embed(ctx context.Context, text string) ([]float32, error) {
	vecs, err := a.e.Embed(ctx, []string{text})
	if err != nil {
		return nil, err
	}
	if len(vecs) == 0 || len(vecs[0]) == 0 {
		return nil, fmt.Errorf("ckv embedder returned empty vector for intent text")
	}
	return vecs[0], nil
}

func loadConfig(path string) (*config.Config, error) {
	if path == "" {
		c := config.Default()
		if err := c.Validate(); err != nil {
			return nil, err
		}
		return c, nil
	}
	return config.Load(path)
}

func loadRuleset(path string) (*config.SanitizeRuleset, error) {
	if path == "" {
		// Minimal NOOP ruleset for dev. Production deployments MUST point
		// Sanitize.RulesPath at the project baseline.
		rs := &config.SanitizeRuleset{
			Version: 1,
			Rules: []config.SanitizeRule{{
				ID: "NOOP", Pattern: `__no_match__`, Action: "drop", Severity: config.SeverityLow,
			}},
		}
		if err := rs.Validate(); err != nil {
			return nil, err
		}
		return rs, nil
	}
	return config.LoadSanitizeRuleset(path)
}

func buildComposer(
	ctx context.Context,
	ckg ckgclient.Client,
	ckv ckvclient.Client,
	embedder intent.Embedder,
	fetcher budget.BodyFetcher,
	ruleset *config.SanitizeRuleset,
	vocabResolver *vocab.Resolver,
	fp *footprint.Logger,
) (*composer.Composer, error) {
	ic, err := intent.New(ctx, embedder, intent.WithFootprint(fp))
	if err != nil {
		return nil, fmt.Errorf("intent.New: %w", err)
	}
	stage1Opts := []stage1.Option{stage1.WithFootprint(fp)}
	if vocabResolver != nil {
		stage1Opts = append(stage1Opts, stage1.WithVocab(vocabResolver))
	}
	s1, err := stage1.New(ckv, ckg, stage1Opts...)
	if err != nil {
		return nil, fmt.Errorf("stage1.New: %w", err)
	}
	s2, err := stage2.New(ckg, stage2.WithFootprint(fp))
	if err != nil {
		return nil, fmt.Errorf("stage2.New: %w", err)
	}
	s3, err := stage3.New(ckg, stage3.WithFootprint(fp))
	if err != nil {
		return nil, fmt.Errorf("stage3.New: %w", err)
	}
	b, err := budget.New(fetcher, budget.WithFootprint(fp))
	if err != nil {
		return nil, fmt.Errorf("budget.New: %w", err)
	}
	san, err := sanitize.New(ruleset, sanitize.WithFootprint(fp))
	if err != nil {
		return nil, fmt.Errorf("sanitize.New: %w", err)
	}
	return composer.New(ic, s1, s2, s3, b, san,
		composer.WithBuilderVersion(builderVersion),
		composer.WithFootprint(fp),
	)
}

// buildFootprint constructs a footprint.Logger based on logging config.
//
// Output policy: when LoggingConfig.FootprintDir is set, events go to
// <dir>/cks-mcp.jsonl (rotation deferred to ops). When empty, events go
// to stderr — MCP stdio reserves stdout for JSON-RPC frames, so stderr
// is the only safe in-band sink.
//
// Returns a Logger plus a closer the caller defers; the closer flushes
// any buffered records and closes the file when applicable.
func buildFootprint(cfg config.LoggingConfig) (*footprint.Logger, func() error, error) {
	mode := footprint.ModeProd
	if cfg.Mode == "dev" {
		mode = footprint.ModeDev
	}
	level := footprint.Level(cfg.Level)
	if level == "" {
		level = footprint.LevelInfo
	}

	var (
		writer     io.Writer = os.Stderr
		fileCloser io.Closer
	)
	if cfg.FootprintDir != "" {
		if err := os.MkdirAll(cfg.FootprintDir, 0o755); err != nil {
			return nil, func() error { return nil }, fmt.Errorf("mkdir %q: %w", cfg.FootprintDir, err)
		}
		f, err := os.OpenFile(filepath.Join(cfg.FootprintDir, "cks-mcp.jsonl"),
			os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, func() error { return nil }, err
		}
		writer = f
		fileCloser = f
	}

	fp, err := footprint.New(footprint.Config{
		Writer: writer,
		Mode:   mode,
		Level:  level,
	})
	if err != nil {
		if fileCloser != nil {
			_ = fileCloser.Close()
		}
		return nil, func() error { return nil }, err
	}
	closer := func() error {
		err := fp.Sync()
		if fileCloser != nil {
			if cerr := fileCloser.Close(); err == nil {
				err = cerr
			}
		}
		return err
	}
	return fp, closer, nil
}
