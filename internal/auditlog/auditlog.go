// Package auditlog provides append-only structured security/decision records.
//
// Unlike footprint (which may be sampled and is intended for debugging/perf),
// audit records are intended to be immutable, tamper-evident, and complete.
// Typical audit events: capability denial, sanitization redaction, policy
// rule hit, evidence pack release, dry-run boundary crossing.
//
// Records are emitted as JSON Lines. A future tamper-evidence pass (Phase 3+)
// will chain SHA-256 hashes across consecutive records; the Record struct
// reserves PrevHash/Hash fields for forward compatibility, but the v0
// implementation leaves them empty.
package auditlog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/0xmhha/code-knowledge-system/internal/envelope"
)

// Decision categorizes the disposition of an audited action.
type Decision string

const (
	DecisionAllow  Decision = "allow"
	DecisionDeny   Decision = "deny"
	DecisionRedact Decision = "redact"
	// DecisionAdvisory marks records that are informational only (no policy
	// gate involved). Useful for tracking sanitize-rule hits that don't
	// block release.
	DecisionAdvisory Decision = "advisory"
)

// Record is one audit log entry.
type Record struct {
	Timestamp time.Time      `json:"ts"`
	TraceID   string         `json:"trace_id,omitempty"`
	RunID     string         `json:"run_id,omitempty"`
	Event     string         `json:"event"`
	Actor     string         `json:"actor,omitempty"`    // who/what initiated (e.g. "cks.composer", "user")
	Resource  string         `json:"resource,omitempty"` // what was acted upon (e.g. "tool:cks.context.get_for_task")
	Decision  Decision       `json:"decision,omitempty"`
	Reason    string         `json:"reason,omitempty"` // free-text, e.g. "rule_id=SANITIZE_PII_EMAIL"
	Fields    map[string]any `json:"fields,omitempty"`
	// PrevHash/Hash reserved for tamper-evidence chain. v0 leaves empty.
	PrevHash string `json:"prev_hash,omitempty"`
	Hash     string `json:"hash,omitempty"`
}

// Logger writes Records as JSON Lines to an underlying writer. All methods
// are safe for concurrent use.
type Logger struct {
	mu     sync.Mutex
	w      io.Writer
	closer io.Closer
}

// New constructs a Logger writing to w. The caller retains ownership of w.
func New(w io.Writer) *Logger {
	return &Logger{w: w}
}

// Open creates a Logger that appends to path, creating the file if needed.
// The file is closed when Logger.Close is called.
func Open(path string) (*Logger, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("auditlog: open %q: %w", path, err)
	}
	return &Logger{w: f, closer: f}, nil
}

// Record appends r to the log, filling in Timestamp/TraceID/RunID from ctx
// when the corresponding fields on r are zero. Returns ErrNoEvent if r.Event
// is empty (audit records must always carry an event name).
func (l *Logger) Record(ctx context.Context, r Record) error {
	if l == nil {
		return nil
	}
	if r.Event == "" {
		return ErrNoEvent
	}

	if r.Timestamp.IsZero() {
		r.Timestamp = time.Now().UTC()
	}
	if r.TraceID == "" {
		r.TraceID = envelope.TraceID(ctx)
	}
	if r.RunID == "" {
		r.RunID = envelope.RunID(ctx)
	}

	buf, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("auditlog: marshal: %w", err)
	}
	buf = append(buf, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	if _, err := l.w.Write(buf); err != nil {
		return fmt.Errorf("auditlog: write: %w", err)
	}
	return nil
}

// Close releases any owned writer. Safe to call multiple times.
func (l *Logger) Close() error {
	if l == nil || l.closer == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	err := l.closer.Close()
	l.closer = nil
	return err
}

// ErrNoEvent is returned by Record when r.Event is empty.
var ErrNoEvent = errors.New("auditlog: record missing event name")
