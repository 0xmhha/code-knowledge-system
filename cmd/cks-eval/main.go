// Command cks-eval runs cks retrieval-quality scenarios and emits a
// JSON metric report.
//
// Phase E (Slim, Layer 1) measures cks's evidence-pack quality without
// invoking an LLM:
//
//   - Loads YAML scenarios from -scenarios (file or directory of *.yaml).
//   - Spawns cks-mcp [-config <path>] via mcp-go stdio.
//   - For each scenario, calls cks.context.get_for_task and computes
//     file precision/recall against the scenario's expected_citations.
//   - Folds per-run values into a median per scenario.
//   - Writes a Report JSON to -output (default stdout).
//
// LLM-with-cks metrics (AST diff, semantic diff, test pass rate, PR
// split quality) live in a future Core slice that adds cli-wrapper
// integration.
//
// Usage:
//
//	cks-eval -scenarios ./eval/scenarios/stablenet-pr70.yaml
//	cks-eval -scenarios ./eval/scenarios/ -cks-mcp ./bin/cks-mcp \
//	         -config ./policies/cks.yaml.example -output eval/reports/run.json
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
	"strings"
	"syscall"
	"time"

	"github.com/0xmhha/code-knowledge-system/internal/eval"
)

var builderVersion = "cks-eval/0.0.1-dev"

func main() {
	scenarios := flag.String("scenarios", "", "scenario YAML file or directory (required)")
	mcpBinary := flag.String("cks-mcp", "", "path to cks-mcp binary (empty = look up cks-mcp on $PATH)")
	mcpConfig := flag.String("config", "", "path to cks.yaml forwarded to cks-mcp")
	output := flag.String("output", "", "write report to this file (empty = stdout)")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(builderVersion)
		return
	}
	if *scenarios == "" {
		log.Println("cks-eval: -scenarios is required")
		os.Exit(2)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx, *scenarios, *mcpBinary, *mcpConfig, *output); err != nil {
		log.Printf("cks-eval: %v", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, scenariosPath, mcpBinary, mcpConfig, outputPath string) error {
	paths, err := collectScenarioPaths(scenariosPath)
	if err != nil {
		return fmt.Errorf("collect scenarios: %w", err)
	}
	if len(paths) == 0 {
		return fmt.Errorf("no scenarios found at %q", scenariosPath)
	}

	runner, err := eval.NewRunner(ctx, eval.RunnerOpts{
		CKSMCPBinary: mcpBinary,
		CKSMCPConfig: mcpConfig,
	})
	if err != nil {
		return fmt.Errorf("start runner: %w", err)
	}
	defer func() { _ = runner.Close() }()

	report := eval.NewReport(builderVersion)
	for _, p := range paths {
		s, err := eval.LoadScenario(p)
		if err != nil {
			return fmt.Errorf("load %q: %w", p, err)
		}
		result, err := runner.Execute(ctx, s)
		if err != nil {
			return fmt.Errorf("execute %q: %w", p, err)
		}
		report.Results = append(report.Results, *result)
	}
	report.FinishedAt = time.Now().UTC()

	out, closer, err := openOutput(outputPath)
	if err != nil {
		return fmt.Errorf("open output: %w", err)
	}
	defer func() { _ = closer() }()

	return eval.WriteJSON(out, report)
}

// collectScenarioPaths accepts either a single YAML file or a
// directory containing *.yaml / *.yml files. Returns paths sorted
// lexicographically so report ordering is deterministic per invocation.
func collectScenarioPaths(p string) ([]string, error) {
	info, err := os.Stat(p)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return []string{p}, nil
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		out = append(out, filepath.Join(p, name))
	}
	if len(out) == 0 {
		return nil, errors.New("directory contains no *.yaml / *.yml files")
	}
	return out, nil
}

func openOutput(path string) (io.Writer, func() error, error) {
	if path == "" {
		return os.Stdout, func() error { return nil }, nil
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, err
	}
	return f, f.Close, nil
}
