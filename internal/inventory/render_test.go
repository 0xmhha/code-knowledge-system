package inventory

import (
	"strings"
	"testing"
)

// TestRenderInventoryMD_PreservesFreeformProse verifies the renderer
// rewrites only the four known table sections and leaves every other
// section (Conventions, Pending work, prologue text) byte-identical.
// This is the contract that lets authors keep hand-curated notes in
// inventory.md without losing them on every entry-verify run.
func TestRenderInventoryMD_PreservesFreeformProse(t *testing.T) {
	t.Parallel()

	p := &Project{
		Subsystems:     map[string]Subsystem{"A1": {ID: "A1", Name: "Sample"}},
		SubsystemOrder: []string{"A1"},
		Entries: map[string]Entry{
			"A1.foo.bar": {
				ID: "A1.foo.bar", Subsystem: "A1", KnowledgeType: "B1",
				Title: "Sample title", Status: "verified", Priority: "P0",
			},
		},
	}

	const input = `# Sample inventory

Some prologue text here.

## Status summary

| Status | Count |
|---|---|
| verified | 999 |

## Conventions

- All entries follow shared/SCHEMA.md.
- This bullet must survive.

## Subsystem coverage

| Subsystem | Name | Entries (verified / total) |
|---|---|---|
| A1 | Old Name | 0 / 999 |

## Pending work

A freeform paragraph that the renderer must not touch.
`
	got := RenderInventoryMD(input, p)

	// Counts updated.
	if !strings.Contains(got, "| verified | 1 |") {
		t.Errorf("Status summary not updated:\n%s", got)
	}
	if !strings.Contains(got, "| A1 | Sample | 1 / 1 |") {
		t.Errorf("Subsystem coverage not updated:\n%s", got)
	}

	// Freeform sections preserved verbatim.
	for _, must := range []string{
		"Some prologue text here.",
		"- All entries follow shared/SCHEMA.md.",
		"- This bullet must survive.",
		"A freeform paragraph that the renderer must not touch.",
	} {
		if !strings.Contains(got, must) {
			t.Errorf("renderer dropped required text %q\n%s", must, got)
		}
	}

	// Stale counts gone.
	if strings.Contains(got, "999") {
		t.Errorf("stale '999' from old tables still present:\n%s", got)
	}
}
