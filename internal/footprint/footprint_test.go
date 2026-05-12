package footprint

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/0xmhha/code-knowledge-system/internal/envelope"
)

func newTestLogger(t *testing.T, level Level) (*Logger, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	l, err := New(Config{Writer: &buf, Mode: ModeProd, Level: level})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l, &buf
}

func decode(t *testing.T, line string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		t.Fatalf("decode %q: %v", line, err)
	}
	return m
}

func TestEvent_EnvelopeFieldsAttached(t *testing.T) {
	t.Parallel()
	l, buf := newTestLogger(t, LevelInfo)

	ctx := envelope.WithTraceID(context.Background(), "trace-abc")
	ctx = envelope.WithRunID(ctx, "run-xyz")
	ctx = envelope.WithDryRun(ctx, true)

	l.Event(ctx, "ckg.query", zap.Int("result_count", 7))
	_ = l.Sync()

	rec := decode(t, strings.TrimSpace(buf.String()))

	checks := map[string]any{
		"event":        "ckg.query",
		"trace_id":     "trace-abc",
		"run_id":       "run-xyz",
		"dry_run":      true,
		"result_count": float64(7),
	}
	for k, want := range checks {
		if got := rec[k]; got != want {
			t.Errorf("field %q = %v (%T), want %v (%T)", k, got, got, want, want)
		}
	}
}

func TestEvent_OmitsEmptyEnvelopeFields(t *testing.T) {
	t.Parallel()
	l, buf := newTestLogger(t, LevelInfo)
	l.Event(context.Background(), "ckg.query")
	_ = l.Sync()

	rec := decode(t, strings.TrimSpace(buf.String()))
	for _, k := range []string{"trace_id", "run_id", "dry_run"} {
		if _, ok := rec[k]; ok {
			t.Errorf("field %q should be omitted when unset", k)
		}
	}
}

func TestEvent_LevelFiltering(t *testing.T) {
	t.Parallel()
	l, buf := newTestLogger(t, LevelInfo)
	l.Debug(context.Background(), "ckg.subquery")
	_ = l.Sync()
	if buf.Len() != 0 {
		t.Fatalf("debug emitted under info level: %q", buf.String())
	}
}

func TestError_AttachesErrField(t *testing.T) {
	t.Parallel()
	l, buf := newTestLogger(t, LevelInfo)
	l.Error(context.Background(), "ckv.embed", errors.New("model unavailable"))
	_ = l.Sync()
	rec := decode(t, strings.TrimSpace(buf.String()))
	if rec["error"] != "model unavailable" {
		t.Fatalf("error field = %v, want model unavailable", rec["error"])
	}
	if rec["level"] != "error" {
		t.Fatalf("level = %v, want error", rec["level"])
	}
}

func TestNew_InvalidLevel(t *testing.T) {
	t.Parallel()
	if _, err := New(Config{Level: "verbose"}); err == nil {
		t.Fatal("expected error for invalid level")
	}
}

func TestNew_InvalidMode(t *testing.T) {
	t.Parallel()
	if _, err := New(Config{Mode: Mode("bogus")}); err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestDiscard_NoPanic(t *testing.T) {
	t.Parallel()
	Discard.Event(context.Background(), "x")
	Discard.Debug(context.Background(), "x")
	Discard.Warn(context.Background(), "x")
	Discard.Error(context.Background(), "x", errors.New("e"))
	_ = Discard.Sync()
	_ = Discard.Close()
}

func TestNilReceiver_NoPanic(t *testing.T) {
	t.Parallel()
	var l *Logger
	l.Event(context.Background(), "x")
	l.Debug(context.Background(), "x")
	l.Warn(context.Background(), "x")
	l.Error(context.Background(), "x", errors.New("e"))
	if err := l.Sync(); err != nil {
		t.Fatalf("nil Sync: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("nil Close: %v", err)
	}
}

func TestNewFile_WritesAndCloses(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "fp.jsonl")
	l, err := NewFile(path, Config{Mode: ModeProd, Level: LevelInfo})
	if err != nil {
		t.Fatalf("NewFile: %v", err)
	}
	l.Event(context.Background(), "ckg.query", zap.Int("k", 1))
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !strings.Contains(string(data), `"event":"ckg.query"`) {
		t.Fatalf("file missing event: %q", data)
	}
}
