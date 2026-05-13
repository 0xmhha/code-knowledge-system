package sanitize

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/0xmhha/code-knowledge-system/internal/config"
	"github.com/0xmhha/code-knowledge-system/internal/footprint"
	"github.com/0xmhha/code-knowledge-system/pkg/contract"
)

// --- helpers ---

func cit(file string, start, end int) contract.Citation {
	return contract.Citation{File: file, StartLine: start, EndLine: end, CommitHash: "abc"}
}

// ruleset builds an inline SanitizeRuleset with the given rules. Calls
// Validate to compile regexes (real usage path).
func ruleset(t *testing.T, rules ...config.SanitizeRule) *config.SanitizeRuleset {
	t.Helper()
	rs := &config.SanitizeRuleset{
		Version: 1,
		Rules:   rules,
	}
	if err := rs.Validate(); err != nil {
		t.Fatalf("ruleset.Validate: %v", err)
	}
	return rs
}

func rule(id, pattern string, action contract.RedactionAction) config.SanitizeRule {
	return config.SanitizeRule{
		ID:       id,
		Pattern:  pattern,
		Action:   action,
		Severity: config.SeverityHigh,
	}
}

// --- New / construction ---

func TestNew_NilRulesetErrors(t *testing.T) {
	t.Parallel()
	_, err := New(nil)
	if err == nil {
		t.Fatal("expected error for nil ruleset")
	}
}

func TestNew_EmptyMaskTokenErrors(t *testing.T) {
	t.Parallel()
	rs := ruleset(t, rule("R1", "foo", contract.RedactionMask))
	_, err := New(rs, WithConfig(Config{MaskToken: ""}))
	if err == nil {
		t.Fatal("expected error for empty MaskToken")
	}
}

func TestNew_PartitionsRulesByAction(t *testing.T) {
	t.Parallel()
	rs := ruleset(t,
		rule("FC", "secret", contract.RedactionFailClosed),
		rule("D1", "drop1", contract.RedactionDrop),
		rule("D2", "drop2", contract.RedactionDrop),
		rule("M1", "mask1", contract.RedactionMask),
	)
	e, err := New(rs)
	if err != nil {
		t.Fatal(err)
	}
	if len(e.failClosedRules) != 1 || len(e.dropRules) != 2 || len(e.maskRules) != 1 {
		t.Errorf("partition: fc=%d drop=%d mask=%d, want 1/2/1",
			len(e.failClosedRules), len(e.dropRules), len(e.maskRules))
	}
}

// --- Sanitize / happy paths ---

func TestSanitize_EmptyItems(t *testing.T) {
	t.Parallel()
	rs := ruleset(t, rule("R1", "foo", contract.RedactionDrop))
	e, _ := New(rs)
	out, err := e.Sanitize(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 0 || len(out.Redactions) != 0 || out.FailClosed {
		t.Errorf("expected empty output, got %+v", out)
	}
}

func TestSanitize_NoMatchesPassesThrough(t *testing.T) {
	t.Parallel()
	rs := ruleset(t, rule("R1", "secretpattern", contract.RedactionDrop))
	e, _ := New(rs)

	items := []Sanitizable{
		{Citation: cit("a.go", 1, 10), Body: "clean code"},
		{Citation: cit("b.go", 1, 10), Body: "more clean code"},
	}
	out, _ := e.Sanitize(context.Background(), items)
	if len(out.Items) != 2 {
		t.Fatalf("Items count = %d, want 2", len(out.Items))
	}
	for i, item := range out.Items {
		if item.Dropped {
			t.Errorf("Items[%d] should not be dropped", i)
		}
		if item.Body != items[i].Body {
			t.Errorf("Items[%d].Body mutated: got %q, want %q", i, item.Body, items[i].Body)
		}
	}
	if len(out.Redactions) != 0 {
		t.Errorf("Redactions = %v, want empty", out.Redactions)
	}
}

// --- Drop rule ---

func TestSanitize_DropRuleClearsBody(t *testing.T) {
	t.Parallel()
	rs := ruleset(t, rule("APIKEY", `sk-[a-z]{4}`, contract.RedactionDrop))
	e, _ := New(rs)

	items := []Sanitizable{
		{Citation: cit("a.go", 1, 10), Body: `apiKey := "sk-abcd"`},
	}
	out, _ := e.Sanitize(context.Background(), items)

	if !out.Items[0].Dropped {
		t.Fatal("expected Dropped=true")
	}
	if out.Items[0].Body != "" {
		t.Errorf("Body = %q, want empty (dropped)", out.Items[0].Body)
	}
	if len(out.Items[0].Redactions) != 1 || out.Items[0].Redactions[0].RuleID != "APIKEY" {
		t.Errorf("Redactions = %+v", out.Items[0].Redactions)
	}
	if len(out.Redactions) != 1 {
		t.Errorf("flat Redactions count = %d, want 1", len(out.Redactions))
	}
}

func TestSanitize_DropRecordsAllRuleHits(t *testing.T) {
	t.Parallel()
	// Two different drop rules both match on the same body.
	rs := ruleset(t,
		rule("DROP_A", "alpha", contract.RedactionDrop),
		rule("DROP_B", "beta", contract.RedactionDrop),
	)
	e, _ := New(rs)
	items := []Sanitizable{
		{Citation: cit("a.go", 1, 10), Body: "alpha and beta in here"},
	}
	out, _ := e.Sanitize(context.Background(), items)

	if !out.Items[0].Dropped {
		t.Fatal("expected Dropped=true")
	}
	if len(out.Items[0].Redactions) != 2 {
		t.Errorf("expected 2 redactions (both rules), got %d", len(out.Items[0].Redactions))
	}
}

// --- Mask rule ---

func TestSanitize_MaskReplacesMatches(t *testing.T) {
	t.Parallel()
	rs := ruleset(t, rule("EMAIL", `[a-z]+@example\.com`, contract.RedactionMask))
	e, _ := New(rs)
	items := []Sanitizable{
		{Citation: cit("a.go", 1, 10), Body: "user@example.com and admin@example.com"},
	}
	out, _ := e.Sanitize(context.Background(), items)

	if out.Items[0].Dropped {
		t.Error("mask should not drop")
	}
	if strings.Contains(out.Items[0].Body, "user@example.com") {
		t.Errorf("body still contains unmasked email: %q", out.Items[0].Body)
	}
	if !strings.Contains(out.Items[0].Body, DefaultMaskToken) {
		t.Errorf("body missing mask token: %q", out.Items[0].Body)
	}
	// One redaction (the rule), with count=2 in Excerpt.
	if len(out.Items[0].Redactions) != 1 {
		t.Fatalf("Redactions count = %d, want 1", len(out.Items[0].Redactions))
	}
	if !strings.Contains(out.Items[0].Redactions[0].Excerpt, "2 occurrence") {
		t.Errorf("Excerpt = %q, want count=2 indication", out.Items[0].Redactions[0].Excerpt)
	}
}

func TestSanitize_CustomMaskToken(t *testing.T) {
	t.Parallel()
	rs := ruleset(t, rule("X", `secret`, contract.RedactionMask))
	e, _ := New(rs, WithConfig(Config{MaskToken: "[REDACTED]"}))
	items := []Sanitizable{
		{Citation: cit("a.go", 1, 10), Body: "the secret token"},
	}
	out, _ := e.Sanitize(context.Background(), items)
	if !strings.Contains(out.Items[0].Body, "[REDACTED]") {
		t.Errorf("body = %q, want custom mask", out.Items[0].Body)
	}
}

// --- Fail-closed rule ---

func TestSanitize_FailClosedAborts(t *testing.T) {
	t.Parallel()
	rs := ruleset(t, rule("PRIVATE_KEY", "BEGIN PRIVATE KEY", contract.RedactionFailClosed))
	e, _ := New(rs)
	items := []Sanitizable{
		{Citation: cit("a.go", 1, 10), Body: "clean"},
		{Citation: cit("b.go", 1, 10), Body: "-----BEGIN PRIVATE KEY-----"},
		{Citation: cit("c.go", 1, 10), Body: "should not be processed"},
	}
	out, _ := e.Sanitize(context.Background(), items)

	if !out.FailClosed {
		t.Fatal("expected FailClosed=true")
	}
	if out.FailClosedRule != "PRIVATE_KEY" {
		t.Errorf("FailClosedRule = %q, want PRIVATE_KEY", out.FailClosedRule)
	}
	// Items processed up to and including the offending item; later
	// items are NOT processed.
	if len(out.Items) != 2 {
		t.Errorf("Items count = %d, want 2 (a.go clean + b.go offender)", len(out.Items))
	}
	if len(out.Items[1].Redactions) != 1 {
		t.Errorf("offending item should have 1 Redaction, got %d", len(out.Items[1].Redactions))
	}
}

// --- Precedence ---

func TestSanitize_FailClosedTrumpsDrop(t *testing.T) {
	t.Parallel()
	// Both fail_closed and drop match. fail_closed wins.
	rs := ruleset(t,
		rule("FC", "secret", contract.RedactionFailClosed),
		rule("DROP", "secret", contract.RedactionDrop),
	)
	e, _ := New(rs)
	items := []Sanitizable{{Citation: cit("a.go", 1, 10), Body: "secret data"}}
	out, _ := e.Sanitize(context.Background(), items)
	if !out.FailClosed {
		t.Fatal("expected FailClosed=true (fc takes precedence)")
	}
}

func TestSanitize_DropTrumpsMask(t *testing.T) {
	t.Parallel()
	// Both drop and mask match same body. Drop wins; mask is not even
	// evaluated.
	rs := ruleset(t,
		rule("DROP", "alpha", contract.RedactionDrop),
		rule("MASK", "beta", contract.RedactionMask),
	)
	e, _ := New(rs)
	items := []Sanitizable{
		{Citation: cit("a.go", 1, 10), Body: "alpha and beta together"},
	}
	out, _ := e.Sanitize(context.Background(), items)

	if !out.Items[0].Dropped {
		t.Fatal("expected Dropped")
	}
	// Body is cleared; mask is not applied.
	if out.Items[0].Body != "" {
		t.Errorf("Body = %q, want empty", out.Items[0].Body)
	}
	// Only the DROP redaction is recorded (mask never ran).
	for _, r := range out.Items[0].Redactions {
		if r.RuleID == "MASK" {
			t.Error("MASK rule recorded despite drop preempting it")
		}
	}
}

// --- Redaction shape ---

func TestSanitize_RedactionExcerptNoRawData(t *testing.T) {
	t.Parallel()
	// The Excerpt must never contain the matched secret bytes.
	rs := ruleset(t, rule("R1", `sk-[a-z]+`, contract.RedactionDrop))
	e, _ := New(rs)
	items := []Sanitizable{
		{Citation: cit("a.go", 1, 10), Body: `apiKey := "sk-thisisthesecret"`},
	}
	out, _ := e.Sanitize(context.Background(), items)
	for _, r := range out.Redactions {
		if strings.Contains(r.Excerpt, "thisisthesecret") {
			t.Errorf("Excerpt leaks secret bytes: %q", r.Excerpt)
		}
	}
}

func TestSanitize_RedactionPathIsCitation(t *testing.T) {
	t.Parallel()
	rs := ruleset(t, rule("R1", `foo`, contract.RedactionDrop))
	e, _ := New(rs)
	c := cit("internal/auth.go", 42, 58)
	items := []Sanitizable{{Citation: c, Body: "foo"}}
	out, _ := e.Sanitize(context.Background(), items)
	if out.Redactions[0].Path != c.String() {
		t.Errorf("Path = %q, want %q", out.Redactions[0].Path, c.String())
	}
}

// --- Footprint ---

func TestSanitize_EmitsFootprintEvent(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	fp, err := footprint.New(footprint.Config{Writer: &buf, Mode: footprint.ModeProd, Level: footprint.LevelInfo})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fp.Close() })

	rs := ruleset(t, rule("R1", `secret`, contract.RedactionDrop))
	e, _ := New(rs, WithFootprint(fp))
	items := []Sanitizable{
		{Citation: cit("a.go", 1, 10), Body: "clean"},
		{Citation: cit("b.go", 1, 10), Body: "has secret"},
	}
	_, _ = e.Sanitize(context.Background(), items)
	_ = fp.Sync()

	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
		t.Fatalf("decode footprint: %v", err)
	}
	if rec["event"] != "composer.stage5_sanitized" {
		t.Errorf("event = %v", rec["event"])
	}
	if rec["item_count"].(float64) != 2 {
		t.Errorf("item_count = %v, want 2", rec["item_count"])
	}
	if rec["dropped_count"].(float64) != 1 {
		t.Errorf("dropped_count = %v, want 1", rec["dropped_count"])
	}
	if rec["fail_closed"].(bool) {
		t.Error("fail_closed should be false")
	}
}

func TestSanitize_FootprintReportsFailClosed(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	fp, err := footprint.New(footprint.Config{Writer: &buf, Mode: footprint.ModeProd, Level: footprint.LevelInfo})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fp.Close() })

	rs := ruleset(t, rule("PK", "BEGIN PRIVATE KEY", contract.RedactionFailClosed))
	e, _ := New(rs, WithFootprint(fp))
	items := []Sanitizable{{Citation: cit("a.go", 1, 10), Body: "BEGIN PRIVATE KEY ..."}}
	_, _ = e.Sanitize(context.Background(), items)
	_ = fp.Sync()

	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
		t.Fatalf("decode footprint: %v", err)
	}
	if !rec["fail_closed"].(bool) {
		t.Error("fail_closed = false, want true")
	}
	if rec["fail_closed_rule"] != "PK" {
		t.Errorf("fail_closed_rule = %v, want PK", rec["fail_closed_rule"])
	}
}

// --- Integration with the shipped baseline ruleset ---

func TestSanitize_BaselineRulesetLoads(t *testing.T) {
	t.Parallel()
	rs, err := config.LoadSanitizeRuleset("../../../policies/sanitization_rules.yaml")
	if err != nil {
		t.Fatalf("LoadSanitizeRuleset: %v", err)
	}
	e, err := New(rs)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Baseline should have at least one fail_closed rule.
	if len(e.failClosedRules) == 0 {
		t.Error("baseline ruleset has no fail_closed rules")
	}
	// And NO mask rules (policy: matched data must not cross LLM boundary).
	if len(e.maskRules) != 0 {
		t.Errorf("baseline has %d mask rules; policy says zero", len(e.maskRules))
	}
}

func TestSanitize_BaselineCatchesSampleSecrets(t *testing.T) {
	t.Parallel()
	rs, err := config.LoadSanitizeRuleset("../../../policies/sanitization_rules.yaml")
	if err != nil {
		t.Fatal(err)
	}
	e, _ := New(rs)

	// PRIVATE_KEY_PEM is fail_closed in the baseline.
	items := []Sanitizable{
		{Citation: cit("a.go", 1, 10), Body: "-----BEGIN RSA PRIVATE KEY-----\nMIIE..."},
	}
	out, _ := e.Sanitize(context.Background(), items)
	if !out.FailClosed {
		t.Error("baseline should fail-closed on PRIVATE_KEY_PEM")
	}

	// API_KEY_OPENAI is drop in the baseline.
	itemsAPI := []Sanitizable{
		{Citation: cit("b.go", 1, 10), Body: `apiKey := "sk-abcdefghijklmnopqrstuv"`},
	}
	out2, _ := e.Sanitize(context.Background(), itemsAPI)
	if !out2.Items[0].Dropped {
		t.Error("baseline should drop on API_KEY_OPENAI")
	}
}
