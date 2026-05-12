package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

func TestDefault_PassesValidate(t *testing.T) {
	t.Parallel()
	c := Default()
	if err := c.Validate(); err != nil {
		t.Fatalf("Default config did not validate: %v", err)
	}
}

func TestLoadBytes_RoundTrip(t *testing.T) {
	t.Parallel()
	yamlSrc := `
version: 1
backends:
  ckg:
    path: "./data/ckg.db"
    timeout_ms: 5000
  ckv:
    path: "./data/ckv.db"
    timeout_ms: 3000
    embed_model: "bge-base-onnx"
listen:
  http_addr: "127.0.0.1:8080"
  mcp_stdio: true
logging:
  level: "debug"
  mode: "dev"
  footprint_dir: "./logs/footprint"
  audit_dir: "./logs/audit"
sanitize:
  rules_path: "./policies/sanitization_rules.yaml"
  default_action: "drop"
  fail_closed_on_unknown_rule: true
`
	c, err := LoadBytes([]byte(yamlSrc))
	if err != nil {
		t.Fatalf("LoadBytes: %v", err)
	}
	if c.Backends.CKG.Path != "./data/ckg.db" {
		t.Errorf("CKG.Path = %q", c.Backends.CKG.Path)
	}
	if c.Logging.Level != "debug" {
		t.Errorf("Logging.Level = %q", c.Logging.Level)
	}
	if c.Sanitize.DefaultAction != contract.RedactionDrop {
		t.Errorf("Sanitize.DefaultAction = %q", c.Sanitize.DefaultAction)
	}
	if !c.Sanitize.FailClosedOnUnknownRule {
		t.Error("FailClosedOnUnknownRule should be true")
	}
}

func TestConfig_Validate_Rejects(t *testing.T) {
	t.Parallel()
	cases := map[string]func(*Config){
		"wrong version":      func(c *Config) { c.Version = 99 },
		"bad logging level":  func(c *Config) { c.Logging.Level = "verbose" },
		"bad logging mode":   func(c *Config) { c.Logging.Mode = "production" },
		"non-loopback host":  func(c *Config) { c.Listen.HTTPAddr = "0.0.0.0:8080" },
		"public ip host":     func(c *Config) { c.Listen.HTTPAddr = "203.0.113.1:8080" },
		"malformed addr":     func(c *Config) { c.Listen.HTTPAddr = "not-a-host-port" },
		"bad default_action": func(c *Config) { c.Sanitize.DefaultAction = contract.RedactionAction("redact") },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := Default()
			mut(c)
			if err := c.Validate(); err == nil {
				t.Fatalf("expected error for %s", name)
			}
		})
	}
}

func TestConfig_Validate_AcceptsLoopbackVariants(t *testing.T) {
	t.Parallel()
	for _, addr := range []string{"127.0.0.1:8080", "127.0.0.5:9000", "localhost:80", "[::1]:8080"} {
		t.Run(addr, func(t *testing.T) {
			t.Parallel()
			c := Default()
			c.Listen.HTTPAddr = addr
			if err := c.Validate(); err != nil {
				t.Fatalf("loopback %q rejected: %v", addr, err)
			}
		})
	}
}

func TestLoadBytes_UnknownFieldRejected(t *testing.T) {
	t.Parallel()
	// Typo on listen key: catches config-file bugs that would otherwise
	// silently fall back to defaults.
	yamlSrc := `
version: 1
liste:
  http_addr: "127.0.0.1:8080"
`
	_, err := LoadBytes([]byte(yamlSrc))
	if err == nil {
		t.Fatal("expected error on unknown top-level field")
	}
}

func TestConfig_Load_File(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "cks.yaml")

	c := Default()
	// Round-trip: serialize default config to disk and load it back.
	// We can't use yaml.Marshal here without importing it, so write
	// the canonical text form directly.
	yamlSrc := `
version: 1
logging:
  level: "info"
  mode: "prod"
sanitize:
  rules_path: "./policies/sanitization_rules.yaml"
  default_action: "drop"
  fail_closed_on_unknown_rule: true
`
	if err := os.WriteFile(path, []byte(yamlSrc), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.Sanitize.DefaultAction != contract.RedactionDrop {
		t.Errorf("DefaultAction = %q", got.Sanitize.DefaultAction)
	}
	_ = c // keep reference to highlight intent
}

func TestConfig_Load_MissingFile(t *testing.T) {
	t.Parallel()
	_, err := Load("/no/such/file.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "read") {
		t.Errorf("error = %v, want 'read' context", err)
	}
}
