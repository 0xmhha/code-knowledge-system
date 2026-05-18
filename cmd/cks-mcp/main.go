// Command cks-mcp serves the cks composer pipeline over an MCP (Model
// Context Protocol) stdio transport.
//
// Phase C.5 (slim) registers two tools: cks.context.get_for_task and
// cks.ops.health. See internal/mcp for handler details.
//
// Backend wiring (post C.1 + C.2):
//   - ckg: if config.Backends.CKG.Path is set, the binary opens a real
//     ckg SQLite store via ckgclient.NewReal. When empty, it falls back
//     to the in-memory Fake.
//   - ckv: if config.Backends.CKV.Path is set, the binary spawns a ckv
//     subprocess (`ckv mcp --out=<path>`) via ckvclient.NewReal and
//     proxies semantic search calls through MCP stdio. When empty, falls
//     back to the in-memory Fake.
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

	ckv, ckvCloser, err := buildCKVClient(ctx, cfg.Backends.CKV)
	if err != nil {
		return fmt.Errorf("build ckv client: %w", err)
	}
	defer func() { _ = ckvCloser() }()

	fp, fpCloser, err := buildFootprint(cfg.Logging)
	if err != nil {
		return fmt.Errorf("build footprint: %w", err)
	}
	defer func() { _ = fpCloser() }()

	embedder := &intent.FakeEmbedder{Dim: 32}
	fetcher := &budget.FakeFetcher{Bodies: map[string]string{}}

	c, err := buildComposer(ctx, ckg, ckv, embedder, fetcher, ruleset, fp)
	if err != nil {
		return fmt.Errorf("build composer: %w", err)
	}

	deps := cksmcp.Deps{
		Composer:       c,
		CKG:            ckg,
		CKV:            ckv,
		BuilderVersion: builderVersion,
	}
	return cksmcp.Run(ctx, deps)
}

// buildCKGClient picks the real adapter when a path is configured and
// falls back to the in-memory Fake otherwise. The returned closer should
// be deferred by the caller; the Fake's closer is a no-op.
func buildCKGClient(path string) (ckgclient.Client, func() error, error) {
	if path == "" {
		f := &ckgclient.Fake{HealthVal: ckgclient.Health{Reachable: true, SchemaVersion: "fake-phase0"}}
		return f, func() error { return nil }, nil
	}
	real, err := ckgclient.NewReal(path)
	if err != nil {
		return nil, func() error { return nil }, err
	}
	return real, real.Close, nil
}

// buildCKVClient picks the real ckv adapter (subprocess MCP proxy) when
// a data path is configured and falls back to the in-memory Fake
// otherwise. NewReal spawns the ckv binary and runs the MCP initialize
// handshake; failures here surface before the cks-mcp server starts
// accepting stdio frames.
func buildCKVClient(ctx context.Context, cfg config.CKVConfig) (ckvclient.Client, func() error, error) {
	if cfg.Path == "" {
		f := &ckvclient.Fake{HealthVal: ckvclient.Health{Reachable: true, StatsHash: "fake-phase0"}}
		return f, func() error { return nil }, nil
	}
	real, err := ckvclient.NewReal(ctx, ckvclient.RealOpts{
		BinaryPath: cfg.BinaryPath,
		DataPath:   cfg.Path,
		Embedder:   cfg.EmbedModel,
	})
	if err != nil {
		return nil, func() error { return nil }, err
	}
	return real, real.Close, nil
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
	fp *footprint.Logger,
) (*composer.Composer, error) {
	ic, err := intent.New(ctx, embedder, intent.WithFootprint(fp))
	if err != nil {
		return nil, fmt.Errorf("intent.New: %w", err)
	}
	s1, err := stage1.New(ckv, ckg, stage1.WithFootprint(fp))
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
		writer io.Writer = os.Stderr
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
