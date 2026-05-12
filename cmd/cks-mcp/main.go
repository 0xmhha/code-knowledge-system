// Command cks-mcp serves the cks MCP surface over stdio JSON-RPC.
//
// Status: stub (Phase 0 scaffold). Real MCP wiring lands in Phase C.5.
package main

import (
	"fmt"
	"os"
)

const version = "0.0.1-dev"

func main() {
	fmt.Fprintf(os.Stderr, "cks-mcp %s — MCP surface not yet implemented (Phase C.5)\n", version)
	os.Exit(1)
}
