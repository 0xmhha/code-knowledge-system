package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

const validRulesetYAML = `
version: 1
rules:
  - id: API_KEY_OPENAI
    description: "OpenAI API key"
    pattern: '\bsk-[A-Za-z0-9_\-]{20,}\b'
    action: drop
    severity: critical
  - id: PRIVATE_KEY_PEM
    description: "PEM private key block"
    pattern: '-----BEGIN ((RSA|EC|OPENSSH|DSA) )?PRIVATE KEY-----'
    action: fail_closed
    severity: critical
`

func TestLoadSanitizeRulesetBytes_OK(t *testing.T) {
	t.Parallel()
	rs, err := LoadSanitizeRulesetBytes([]byte(validRulesetYAML))
	if err != nil {
		t.Fatalf("LoadSanitizeRulesetBytes: %v", err)
	}
	if len(rs.Rules) != 2 {
		t.Fatalf("Rules count = %d, want 2", len(rs.Rules))
	}
	if rs.Rules[0].Regexp() == nil {
		t.Error("first rule regex not compiled")
	}
	if !rs.Rules[0].Regexp().MatchString("apiKey := \"sk-abcdefghijklmnopqrstuv\"") {
		t.Error("OpenAI pattern did not match expected secret")
	}
}

func TestSanitizeRuleset_Lookup(t *testing.T) {
	t.Parallel()
	rs, err := LoadSanitizeRulesetBytes([]byte(validRulesetYAML))
	if err != nil {
		t.Fatal(err)
	}
	if got := rs.Lookup("API_KEY_OPENAI"); got == nil || got.ID != "API_KEY_OPENAI" {
		t.Errorf("Lookup(API_KEY_OPENAI) = %+v", got)
	}
	if got := rs.Lookup("DOES_NOT_EXIST"); got != nil {
		t.Errorf("Lookup miss returned %+v, want nil", got)
	}
}

func TestSanitizeRuleset_FailClosedRules(t *testing.T) {
	t.Parallel()
	rs, err := LoadSanitizeRulesetBytes([]byte(validRulesetYAML))
	if err != nil {
		t.Fatal(err)
	}
	fc := rs.FailClosedRules()
	if len(fc) != 1 {
		t.Fatalf("FailClosedRules count = %d, want 1", len(fc))
	}
	if fc[0].ID != "PRIVATE_KEY_PEM" {
		t.Errorf("FailClosedRules[0] = %q, want PRIVATE_KEY_PEM", fc[0].ID)
	}
}

func TestSanitize_Validate_Rejects(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"wrong version": `
version: 99
rules:
  - id: X
    pattern: 'x'
    action: drop
    severity: high
`,
		"empty rules": `
version: 1
rules: []
`,
		"duplicate id": `
version: 1
rules:
  - id: DUP
    pattern: 'a'
    action: drop
    severity: low
  - id: DUP
    pattern: 'b'
    action: drop
    severity: low
`,
		"invalid regex": `
version: 1
rules:
  - id: BAD
    pattern: '['
    action: drop
    severity: low
`,
		"unknown action": `
version: 1
rules:
  - id: X
    pattern: 'x'
    action: redact_lol
    severity: low
`,
		"unknown severity": `
version: 1
rules:
  - id: X
    pattern: 'x'
    action: drop
    severity: catastrophic
`,
		"missing id": `
version: 1
rules:
  - id: ""
    pattern: 'x'
    action: drop
    severity: low
`,
		"missing pattern": `
version: 1
rules:
  - id: X
    pattern: ""
    action: drop
    severity: low
`,
	}
	for name, yamlSrc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, err := LoadSanitizeRulesetBytes([]byte(yamlSrc))
			if err == nil {
				t.Fatalf("expected error for %s", name)
			}
		})
	}
}

func TestSanitize_Load_File(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.yaml")
	if err := os.WriteFile(path, []byte(validRulesetYAML), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	rs, err := LoadSanitizeRuleset(path)
	if err != nil {
		t.Fatalf("LoadSanitizeRuleset: %v", err)
	}
	if len(rs.Rules) != 2 {
		t.Errorf("Rules count = %d, want 2", len(rs.Rules))
	}
}

func TestSanitize_Load_MissingFile(t *testing.T) {
	t.Parallel()
	_, err := LoadSanitizeRuleset("/no/such/rules.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "read") {
		t.Errorf("error = %v, want 'read' context", err)
	}
}

func TestRepoBaselineLoads(t *testing.T) {
	t.Parallel()
	// Smoke test: the shipped policies/sanitization_rules.yaml must load
	// and pass validation. Fail loudly if a future PR breaks the baseline.
	rs, err := LoadSanitizeRuleset("../../policies/sanitization_rules.yaml")
	if err != nil {
		t.Fatalf("policies/sanitization_rules.yaml: %v", err)
	}
	// Phase-0 baseline must include at least the private-key fail-closed rule.
	fc := rs.FailClosedRules()
	if len(fc) == 0 {
		t.Fatal("baseline ruleset has no fail_closed rules; private keys must be hard-stopped")
	}
	// And the baseline must NOT use mask (project policy: no LLM-side leakage).
	for _, r := range rs.Rules {
		if r.Action == contract.RedactionMask {
			t.Errorf("baseline rule %q uses mask; policy requires drop or fail_closed", r.ID)
		}
	}
}
