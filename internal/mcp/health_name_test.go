package mcp

import (
	"context"
	"testing"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
)

// cks.ops.health must echo the instance name/description so a caller that
// connected by ip:port can tell WHICH of several cks-mcp instances it reached.
func TestHandleHealth_IncludesInstanceIdentity(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	f.deps.InstanceName = "cks-pr77-2"
	f.deps.InstanceDescription = "go-stablenet pr-77-2 flow index"

	res, err := handleHealth(context.Background(), f.deps, mcpgo.CallToolRequest{})
	if err != nil {
		t.Fatalf("handleHealth: %v", err)
	}
	var out struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := decodeStructured(res, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Name != "cks-pr77-2" {
		t.Errorf("health name = %q, want cks-pr77-2", out.Name)
	}
	if out.Description != "go-stablenet pr-77-2 flow index" {
		t.Errorf("health description = %q", out.Description)
	}
}
