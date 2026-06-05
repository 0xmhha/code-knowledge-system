package inventory

import (
	"path/filepath"
	"testing"
)

// TestLoadProject_CodeRootResolution pins the machine-independent code_root
// resolution: a committed project.yaml may carry "${ENV}" (not a machine path),
// CKS_CODE_ROOT overrides it, and an unset env degrades to an empty code_root
// (anchor checks then skip rather than break).
func TestLoadProject_CodeRootResolution(t *testing.T) {
	root := t.TempDir()
	tree := filepath.Join(root, "tree")
	mustMkdir(t, tree)

	projectDir := filepath.Join(root, "project")
	mustMkdir(t, filepath.Join(projectDir, "entries"))
	mustWrite(t, filepath.Join(projectDir, "project.yaml"),
		"id: s\nname: s\ncode_root: ${TEST_GSN_ROOT}\nschema_version: 1\n")
	mustWrite(t, filepath.Join(projectDir, "subsystems.yaml"),
		"- id: A1\n  name: x\n  description: x\n  code_paths:\n    - .\n")
	mustWrite(t, filepath.Join(projectDir, "entries", "A1.e.f.yaml"),
		"id: A1.e.f\nsubsystem: A1\nknowledge_type: B1\ntitle: T\n"+
			"summary: long enough summary\nstatus: needs_verification\npriority: P0\n")

	// 1. env var in project.yaml is expanded.
	t.Setenv("TEST_GSN_ROOT", tree)
	p, err := LoadProject(projectDir)
	if err != nil {
		t.Fatalf("LoadProject: %v", err)
	}
	if p.CodeRoot != tree {
		t.Errorf("env-expanded code_root = %q, want %q", p.CodeRoot, tree)
	}

	// 2. CKS_CODE_ROOT overrides project.yaml.
	other := filepath.Join(root, "other")
	mustMkdir(t, other)
	t.Setenv("CKS_CODE_ROOT", other)
	p2, err := LoadProject(projectDir)
	if err != nil {
		t.Fatalf("LoadProject (override): %v", err)
	}
	if p2.CodeRoot != other {
		t.Errorf("CKS_CODE_ROOT override = %q, want %q", p2.CodeRoot, other)
	}

	// 3. unset env -> empty code_root (no machine path leaks, checks skip).
	t.Setenv("CKS_CODE_ROOT", "")
	t.Setenv("TEST_GSN_ROOT", "")
	p3, err := LoadProject(projectDir)
	if err != nil {
		t.Fatalf("LoadProject (unset): %v", err)
	}
	if p3.CodeRoot != "" {
		t.Errorf("unset env -> code_root = %q, want empty", p3.CodeRoot)
	}
}
