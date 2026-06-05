package inventory

import (
	"path/filepath"
	"testing"
)

func TestLoadProject_AuthoritativeDocs(t *testing.T) {
	dir := t.TempDir()
	mustMkdir(t, filepath.Join(dir, "entries"))
	mustWrite(t, filepath.Join(dir, "project.yaml"),
		"id: s\nname: s\nschema_version: 1\n"+
			"authoritative_docs:\n"+
			"  - file: CLAUDE.md\n    role: overview\n"+
			"  - file: .claude/docs/wbft-consensus.md\n    role: consensus\n")
	mustWrite(t, filepath.Join(dir, "subsystems.yaml"),
		"- id: A1\n  name: x\n  description: x\n  code_paths:\n    - .\n")
	mustWrite(t, filepath.Join(dir, "entries", "A1.e.f.yaml"),
		"id: A1.e.f\nsubsystem: A1\nknowledge_type: B1\ntitle: T\n"+
			"summary: long enough summary\nstatus: draft\npriority: P0\n")

	p, err := LoadProject(dir)
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}
	if len(p.AuthoritativeDocs) != 2 {
		t.Fatalf("AuthoritativeDocs len = %d, want 2", len(p.AuthoritativeDocs))
	}
	if p.AuthoritativeDocs[0].File != "CLAUDE.md" || p.AuthoritativeDocs[0].Role != "overview" {
		t.Errorf("doc[0] = %+v", p.AuthoritativeDocs[0])
	}
	if p.AuthoritativeDocs[1].File != ".claude/docs/wbft-consensus.md" || p.AuthoritativeDocs[1].Role != "consensus" {
		t.Errorf("doc[1] = %+v", p.AuthoritativeDocs[1])
	}
}
