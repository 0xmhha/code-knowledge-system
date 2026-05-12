// Command cks-eval runs evaluation scenarios against the coding agent.
//
// Drives headless Claude Code via github.com/0xmhha/cli-wrapper, collects diffs,
// compares against human baselines (e.g. go-stablenet PR #70), and emits metric
// reports (file precision/recall, AST/semantic diff similarity, test pass rate,
// PR split quality, token efficiency).
//
// Status: stub (Phase 0 scaffold). Harness lands in Phase E.
package main

import (
	"fmt"
	"os"
)

const version = "0.0.1-dev"

func main() {
	fmt.Fprintf(os.Stderr, "cks-eval %s — evaluation harness not yet implemented (Phase E)\n", version)
	os.Exit(1)
}
