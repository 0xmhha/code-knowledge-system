package observe

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/0xmhha/code-knowledge-system/internal/auditlog"
	"github.com/0xmhha/code-knowledge-system/internal/footprint"
)

func newFP(t *testing.T) (*footprint.Logger, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	l, err := footprint.New(footprint.Config{Writer: &buf, Mode: footprint.ModeProd, Level: footprint.LevelInfo})
	if err != nil {
		t.Fatalf("footprint.New: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l, &buf
}

func newAL() (*auditlog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return auditlog.New(&buf), &buf
}

func TestAudited_WritesBoth(t *testing.T) {
	t.Parallel()
	fp, fpBuf := newFP(t)
	al, alBuf := newAL()

	err := Audited(context.Background(), fp, al, auditlog.Record{
		Event:    "policy.gate",
		Actor:    "cks.composer",
		Resource: "tool:cks.context.get_for_task",
		Decision: auditlog.DecisionAllow,
	}, zap.Int("budget", 8000))
	if err != nil {
		t.Fatalf("Audited: %v", err)
	}
	_ = fp.Sync()

	// footprint record
	var fpRec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(fpBuf.Bytes()), &fpRec); err != nil {
		t.Fatalf("decode footprint: %v", err)
	}
	if fpRec["event"] != "policy.gate" {
		t.Errorf("footprint event = %v, want policy.gate", fpRec["event"])
	}
	if fpRec["actor"] != "cks.composer" {
		t.Errorf("footprint actor = %v, want cks.composer", fpRec["actor"])
	}
	if fpRec["resource"] != "tool:cks.context.get_for_task" {
		t.Errorf("footprint resource = %v", fpRec["resource"])
	}
	if fpRec["decision"] != "allow" {
		t.Errorf("footprint decision = %v, want allow", fpRec["decision"])
	}
	if fpRec["budget"] != float64(8000) {
		t.Errorf("footprint budget = %v, want 8000", fpRec["budget"])
	}

	// audit record
	alLine := strings.TrimSpace(alBuf.String())
	var alRec auditlog.Record
	if err := json.Unmarshal([]byte(alLine), &alRec); err != nil {
		t.Fatalf("decode audit: %v", err)
	}
	if alRec.Event != "policy.gate" || alRec.Decision != auditlog.DecisionAllow {
		t.Errorf("audit record wrong: %+v", alRec)
	}
	if alRec.Hash == "" {
		t.Error("audit Hash not set")
	}
}

func TestAudited_EmptyEventErr(t *testing.T) {
	t.Parallel()
	fp, _ := newFP(t)
	al, alBuf := newAL()

	err := Audited(context.Background(), fp, al, auditlog.Record{})
	if !errors.Is(err, auditlog.ErrNoEvent) {
		t.Fatalf("Audited empty event = %v, want ErrNoEvent", err)
	}
	if alBuf.Len() != 0 {
		t.Fatalf("audit buffer should be empty on error, got %q", alBuf.String())
	}
}

func TestAudited_NilFootprint_Ok(t *testing.T) {
	t.Parallel()
	al, alBuf := newAL()
	err := Audited(context.Background(), nil, al, auditlog.Record{Event: "x"})
	if err != nil {
		t.Fatalf("Audited nil fp: %v", err)
	}
	if alBuf.Len() == 0 {
		t.Fatal("audit not written when fp is nil")
	}
}

func TestAudited_NilAudit_Ok(t *testing.T) {
	t.Parallel()
	fp, fpBuf := newFP(t)
	err := Audited(context.Background(), fp, nil, auditlog.Record{Event: "x"})
	if err != nil {
		t.Fatalf("Audited nil al: %v", err)
	}
	_ = fp.Sync()
	if fpBuf.Len() == 0 {
		t.Fatal("footprint not written when al is nil")
	}
}

func TestAudited_FootprintWrittenEvenIfAuditFails(t *testing.T) {
	t.Parallel()
	fp, fpBuf := newFP(t)
	// failingWriter forces auditlog.Record to fail.
	al := auditlog.New(failingWriter{})

	err := Audited(context.Background(), fp, al, auditlog.Record{Event: "x"})
	if err == nil {
		t.Fatal("Audited expected error from audit write")
	}
	_ = fp.Sync()
	if fpBuf.Len() == 0 {
		t.Fatal("footprint should be written even when audit fails")
	}
}

type failingWriter struct{}

func (failingWriter) Write(p []byte) (int, error) { return 0, errors.New("disk full") }
