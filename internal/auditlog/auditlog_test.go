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
	// Each line must be valid JSON on its own (no interleaving).
	for _, line := range lines {
		if !json.Valid([]byte(line)) {
			t.Fatalf("invalid JSON line: %q", line)
		}
	}
}

func TestOpen_AppendsToFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")

	l1, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := l1.Record(context.Background(), Record{Event: "first"}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	if err := l1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	l2, err := Open(path)
	if err != nil {
		t.Fatalf("Open #2: %v", err)
	}
	if err := l2.Record(context.Background(), Record{Event: "second"}); err != nil {
		t.Fatalf("Record #2: %v", err)
	}
	if err := l2.Close(); err != nil {
		t.Fatalf("Close #2: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("file has %d lines, want 2: %q", len(lines), data)
	}
	if !strings.Contains(lines[0], `"event":"first"`) || !strings.Contains(lines[1], `"event":"second"`) {
		t.Fatalf("append order/content wrong: %q", data)
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
}
