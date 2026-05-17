// Command cks-mcp serves the cks composer pipeline over an MCP (Model
// Context Protocol) stdio transport.
//
// Phase C.5 (slim) registers two tools: cks.context.get_for_task and
// cks.ops.health. See internal/mcp for handler details.
//
// Phase 0 wiring: this binary uses the in-memory ckg/ckv fakes from
// internal/{ckgclient,ckvclient}. Real backend adapters land in Phase
// C.1 (ckg) and C.2 (ckv); the swap is a constructor change in this
// file, not a composer change — the composer depends on the Client
// interfaces, not the implementations.
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
	"log"
	"os"
	"os/signal"
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

	// Phase 0 wiring: fakes for ckg/ckv. Replace with real adapters
	// in Phase C.1/C.2.
	ckg := &ckgclient.Fake{HealthVal: ckgclient.Health{Reachable: true, SchemaVersion: "fake-phase0"}}
	ckv := &ckvclient.Fake{HealthVal: ckvclient.Health{Reachable: true, StatsHash: "fake-phase0"}}
	embedder := &intent.FakeEmbedder{Dim: 32}
	fetcher := &budget.FakeFetcher{Bodies: map[string]string{}}

	c, err := buildComposer(ctx, ckg, ckv, embedder, fetcher, ruleset)
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
) (*composer.Composer, error) {
	ic, err := intent.New(ctx, embedder)
	if err != nil {
		return nil, fmt.Errorf("intent.New: %w", err)
	}
	s1, err := stage1.New(ckv, ckg)
	if err != nil {
		return nil, fmt.Errorf("stage1.New: %w", err)
	}
	s2, err := stage2.New(ckg)
	if err != nil {
		return nil, fmt.Errorf("stage2.New: %w", err)
	}
	s3, err := stage3.New(ckg)
	if err != nil {
		return nil, fmt.Errorf("stage3.New: %w", err)
	}
	b, err := budget.New(fetcher)
	if err != nil {
		return nil, fmt.Errorf("budget.New: %w", err)
	}
	san, err := sanitize.New(ruleset)
	if err != nil {
		return nil, fmt.Errorf("sanitize.New: %w", err)
	}
	return composer.New(ic, s1, s2, s3, b, san, composer.WithBuilderVersion(builderVersion))
}
