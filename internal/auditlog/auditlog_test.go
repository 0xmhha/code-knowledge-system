package auditlog

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/0xmhha/code-knowledge-system/internal/envelope"
)

func decodeOne(t *testing.T, line string) Record {
	t.Helper()
	var r Record
	if err := json.Unmarshal([]byte(line), &r); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return r
}

func TestRecord_BasicFields(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	l := New(&buf)

	ctx := envelope.WithTraceID(context.Background(), "t-1")
	ctx = envelope.WithRunID(ctx, "r-1")

	err := l.Record(ctx, Record{
		Event:    "tool.invoke",
		Actor:    "cks.composer",
		Resource: "tool:cks.context.get_for_task",
		Decision: DecisionAllow,
		Fields:   map[string]any{"budget_tokens": 8000},
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	r := decodeOne(t, strings.TrimSpace(buf.String()))
	if r.Event != "tool.invoke" {
		t.Errorf("Event = %q, want tool.invoke", r.Event)
	}
	if r.TraceID != "t-1" || r.RunID != "r-1" {
		t.Errorf("envelope not propagated: %+v", r)
	}
	if r.Decision != DecisionAllow {
		t.Errorf("Decision = %q, want allow", r.Decision)
	}
	if r.Timestamp.IsZero() {
		t.Error("Timestamp not auto-filled")
	}
	if v, ok := r.Fields["budget_tokens"].(float64); !ok || v != 8000 {
		t.Errorf("Fields.budget_tokens = %v, want 8000", r.Fields["budget_tokens"])
	}
	if r.Hash == "" {
		t.Error("Hash not set")
	}
	if r.PrevHash != "" {
		t.Errorf("PrevHash = %q on first record, want \"\"", r.PrevHash)
	}
}

func TestRecord_NoEventReturnsErr(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	l := New(&buf)
	if err := l.Record(context.Background(), Record{}); err == nil {
		t.Fatal("expected error for empty event")
	}
	if buf.Len() != 0 {
		t.Fatalf("buffer should be empty on error, got: %q", buf.String())
	}
}

func TestRecord_PreservesExplicitTimestamp(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	l := New(&buf)
	want := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	err := l.Record(context.Background(), Record{
		Event:     "sanitize.hit",
		Timestamp: want,
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	r := decodeOne(t, strings.TrimSpace(buf.String()))
	if !r.Timestamp.Equal(want) {
		t.Fatalf("Timestamp = %v, want %v", r.Timestamp, want)
	}
}

func TestRecord_ConcurrentWrites(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	l := New(&buf)

	const n = 200
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		go func(i int) {
			defer wg.Done()
			_ = l.Record(context.Background(), Record{
				Event:  "concurrent.test",
				Fields: map[string]any{"i": i},
			})
		}(i)
	}
	wg.Wait()

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != n {
		t.Fatalf("wrote %d lines, want %d", len(lines), n)
	}
	for _, line := range lines {
		if !json.Valid([]byte(line)) {
			t.Fatalf("invalid JSON line: %q", line)
		}
	}

	// Verify the chain across concurrent writes.
	count, _, err := Verify(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("Verify after concurrent writes: %v", err)
	}
	if count != n {
		t.Fatalf("Verify counted %d records, want %d", count, n)
	}
}

func TestChain_LinksPrevHash(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	l := New(&buf)

	for i := range 5 {
		if err := l.Record(context.Background(), Record{
			Event:  "chain.test",
			Fields: map[string]any{"i": i},
		}); err != nil {
			t.Fatalf("Record %d: %v", i, err)
		}
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 5 {
		t.Fatalf("got %d lines, want 5", len(lines))
	}

	prev := ""
	for i, line := range lines {
		r := decodeOne(t, line)
		if r.PrevHash != prev {
			t.Fatalf("record %d: PrevHash = %q, want %q", i, r.PrevHash, prev)
		}
		if r.Hash == "" {
			t.Fatalf("record %d: Hash empty", i)
		}
		prev = r.Hash
	}
}

func TestVerify_CleanChainPasses(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	l := New(&buf)
	for i := range 10 {
		_ = l.Record(context.Background(), Record{
			Event:  "verify.test",
			Fields: map[string]any{"i": i},
		})
	}
	count, lastHash, err := Verify(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if count != 10 {
		t.Fatalf("count = %d, want 10", count)
	}
	if lastHash != l.LastHash() {
		t.Fatalf("lastHash = %q, logger.LastHash = %q", lastHash, l.LastHash())
	}
}

func TestVerify_DetectsTamper_ChangedField(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	l := New(&buf)
	for i := range 3 {
		_ = l.Record(context.Background(), Record{
			Event:    "policy.hit",
			Decision: DecisionAllow,
			Fields:   map[string]any{"i": i},
		})
	}

	// Tamper: flip Decision on the middle record without re-hashing.
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	r := decodeOne(t, lines[1])
	r.Decision = DecisionDeny // attacker downgrades a deny to allow, etc.
	tampered, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	lines[1] = string(tampered)
	tamperedLog := strings.Join(lines, "\n") + "\n"

	_, _, err = Verify(strings.NewReader(tamperedLog))
	if err == nil {
		t.Fatal("Verify returned nil on tampered chain; want hash mismatch")
	}
	if !strings.Contains(err.Error(), "hash mismatch") {
		t.Fatalf("Verify error = %v, want hash mismatch", err)
	}
}

func TestVerify_DetectsTamper_DeletedRecord(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	l := New(&buf)
	for i := range 3 {
		_ = l.Record(context.Background(), Record{
			Event:  "policy.hit",
			Fields: map[string]any{"i": i},
		})
	}

	// Delete the middle record entirely.
	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	truncated := lines[0] + "\n" + lines[2] + "\n"

	_, _, err := Verify(strings.NewReader(truncated))
	if err == nil {
		t.Fatal("Verify returned nil on truncated chain; want prev_hash mismatch")
	}
	if !strings.Contains(err.Error(), "prev_hash mismatch") {
		t.Fatalf("Verify error = %v, want prev_hash mismatch", err)
	}
}

func TestVerify_EmptyReader(t *testing.T) {
	t.Parallel()
	count, lastHash, err := Verify(strings.NewReader(""))
	if err != nil {
		t.Fatalf("Verify empty: %v", err)
	}
	if count != 0 || lastHash != "" {
		t.Fatalf("count=%d lastHash=%q, want 0/\"\"", count, lastHash)
	}
}

func TestOpen_AppendsAndContinuesChain(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")

	// Session 1: write 3 records.
	l1, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if l1.LastHash() != "" {
		t.Fatalf("LastHash on new file = %q, want \"\"", l1.LastHash())
	}
	for i := range 3 {
		if err := l1.Record(context.Background(), Record{
			Event:  "session1",
			Fields: map[string]any{"i": i},
		}); err != nil {
			t.Fatalf("Record session1.%d: %v", i, err)
		}
	}
	wantLast := l1.LastHash()
	if err := l1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Session 2: open and verify lastHash recovered.
	l2, err := Open(path)
	if err != nil {
		t.Fatalf("Open #2: %v", err)
	}
	if got := l2.LastHash(); got != wantLast {
		t.Fatalf("recovered LastHash = %q, want %q", got, wantLast)
	}

	// Append 2 more.
	for i := range 2 {
		if err := l2.Record(context.Background(), Record{
			Event:  "session2",
			Fields: map[string]any{"i": i},
		}); err != nil {
			t.Fatalf("Record session2.%d: %v", i, err)
		}
	}
	if err := l2.Close(); err != nil {
		t.Fatalf("Close #2: %v", err)
	}

	// Full file chain verifies clean.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	count, _, err := Verify(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Verify across sessions: %v", err)
	}
	if count != 5 {
		t.Fatalf("count = %d, want 5 (3 session1 + 2 session2)", count)
	}
}

func TestNilLogger_NoPanic(t *testing.T) {
	t.Parallel()
	var l *Logger
	if err := l.Record(context.Background(), Record{Event: "x"}); err != nil {
		t.Fatalf("nil Record: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("nil Close: %v", err)
	}
	if got := l.LastHash(); got != "" {
		t.Fatalf("nil LastHash = %q, want \"\"", got)
	}
}
