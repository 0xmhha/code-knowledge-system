// Command cks-agent is the coding agent CLI.
//
// Pipeline: vibe prompt -> requirement extraction -> impact analysis (via cks MCP)
// -> design synthesis (with project skills) -> PR split -> code generation -> tests
// -> verification.
//
// Status: stub (Phase 0 scaffold). Pipeline implementation lands in Phase D.
package main

import (
	"fmt"
	"os"
)

const version = "0.0.1-dev"

func main() {
	fmt.Fprintf(os.Stderr, "cks-agent %s — coding agent not yet implemented (Phase D)\n", version)
	os.Exit(1)
}
