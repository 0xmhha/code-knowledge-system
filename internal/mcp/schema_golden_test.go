package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	mcpserver "github.com/mark3labs/mcp-go/server"
)

// TestSchemaGolden_RegisteredToolsMatchContract is the M2.a conformance gate:
// the cks.* tools the server actually registers must equal — exactly — the
// cks.* tool set in the vendored SSoT fixture (internal/mcp/testdata/
// agent-mcp.schema.json). Adding a tool without updating the fixture (or vice
// versa) fails here, so the agent-facing contract can't drift silently.
func TestSchemaGolden_RegisteredToolsMatchContract(t *testing.T) {
	// 1. Registered cks.* tools (filter out any non-cks tools just in case).
	f := newFixture(t, nil)
	srv := mcpserver.NewMCPServer("cks-test", "0.0.1")
	if err := Register(srv, f.deps); err != nil {
		t.Fatalf("Register: %v", err)
	}
	registered := map[string]struct{}{}
	for name := range srv.ListTools() {
		if strings.HasPrefix(name, "cks.") {
			registered[name] = struct{}{}
		}
	}

	// 2. Contract cks.* tools from the SSoT fixture.
	raw, err := os.ReadFile(filepath.Join("testdata", "agent-mcp.schema.json"))
	if err != nil {
		t.Fatalf("read SSoT fixture: %v", err)
	}
	var schema struct {
		Tools map[string]json.RawMessage `json:"tools"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("parse SSoT fixture: %v", err)
	}
	contract := map[string]struct{}{}
	for name := range schema.Tools {
		if strings.HasPrefix(name, "cks.") {
			contract[name] = struct{}{}
		}
	}

	// 3. Exact set equality — report drift in both directions.
	for name := range registered {
		if _, ok := contract[name]; !ok {
			t.Errorf("registered tool %q is NOT in the SSoT fixture (add it to testdata/agent-mcp.schema.json)", name)
		}
	}
	for name := range contract {
		if _, ok := registered[name]; !ok {
			t.Errorf("SSoT fixture lists %q but the server does not register it (wire it in server.go, or remove it from the fixture)", name)
		}
	}

	if t.Failed() {
		t.Logf("registered: %v", sortedKeys(registered))
		t.Logf("contract:   %v", sortedKeys(contract))
	}
	// Lock the count so a same-sized swap (rename) is also caught by the
	// per-name checks above; this is a fast canary on the total.
	if len(registered) != len(contract) {
		t.Errorf("tool count drift: registered=%d contract=%d", len(registered), len(contract))
	}
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
