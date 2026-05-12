// Package auditlog provides append-only structured security/decision records.
//
// Unlike footprint (which is for debugging/perf observation), audit records
// are intended to be immutable, tamper-evident, and complete. Typical audit
// events: capability denial, sanitization redaction, policy rule hit,
// evidence pack release, dry-run boundary crossing.
//
// Records are emitted as JSON Lines and SHA-256 hash-chained: each record's
// Hash covers the canonical serialization of itself (with Hash zeroed) plus
// its PrevHash. Any in-place edit of an older record breaks the chain at
// that record. Verify scans a log and confirms chain integrity.
//
// Limitations of v0:
//   - The chain provides tamper-evidence, not tamper-prevention. An attacker
//     with write access can rewrite the entire file including hashes; only
//     external (signed/published) anchors close that loop. Phase 3+ may add
//     periodic Merkle anchoring.
//   - On Open, the resume path reads the entire existing file to recover
//     lastHash. For multi-GB logs this is wasteful; a chunked backward scan
//     is a candidate optimization once log sizes warrant it.
package auditlog

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
//
// Struct field order is the canonical serialization order; reordering fields
// is a chain-breaking change. PrevHash and Hash are filled in by Logger
// when the record is recorded; callers should leave them empty.
type Record struct {
	Timestamp time.Time      `json:"ts"`
	TraceID   string         `json:"trace_id,omitempty"`
	RunID     string         `json:"run_id,omitempty"`
	Event     string         `json:"event"`
	Actor     string         `json:"actor,omitempty"`    // who/what initiated
	Resource  string         `json:"resource,omitempty"` // what was acted upon
	Decision  Decision       `json:"decision,omitempty"`
	Reason    string         `json:"reason,omitempty"` // free-text rationale
	Fields    map[string]any `json:"fields,omitempty"`
	PrevHash  string         `json:"prev_hash,omitempty"`
	Hash      string         `json:"hash,omitempty"`
}

// Logger writes Records as hash-chained JSON Lines to an underlying writer.
// All methods are safe for concurrent use.
type Logger struct {
	mu       sync.Mutex
	w        io.Writer
	closer   io.Closer
	lastHash string
}

// New constructs a Logger writing to w with an empty hash chain. The caller
// retains ownership of w. Use Open to continue an existing chain on disk.
func New(w io.Writer) *Logger {
	return &Logger{w: w}
}

// Open creates a Logger that appends to path, creating the file if needed.
// If the file already exists with prior records, the last record's Hash is
// recovered and used as PrevHash for the next emission, continuing the
// chain. The file is closed when Logger.Close is called.
func Open(path string) (*Logger, error) {
	lastHash, err := readLastHash(path)
	if err != nil {
		return nil, fmt.Errorf("auditlog: recover chain at %q: %w", path, err)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("auditlog: open %q: %w", path, err)
	}
	return &Logger{w: f, closer: f, lastHash: lastHash}, nil
}

// Record appends r to the log, filling in Timestamp/TraceID/RunID from ctx
// and computing PrevHash/Hash for chain integrity. Returns ErrNoEvent if
// r.Event is empty (audit records must always carry an event name).
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

	l.mu.Lock()
	defer l.mu.Unlock()

	r.PrevHash = l.lastHash
	hash, err := computeHash(r)
	if err != nil {
		return fmt.Errorf("auditlog: compute hash: %w", err)
	}
	r.Hash = hash

	buf, err := json.Marshal(r)
	if err != nil {
		return fmt.Errorf("auditlog: marshal: %w", err)
	}
	buf = append(buf, '\n')
	if _, err := l.w.Write(buf); err != nil {
		return fmt.Errorf("auditlog: write: %w", err)
	}
	l.lastHash = r.Hash
	return nil
}

// LastHash returns the Hash of the most recently written record, or "" if
// the log is empty. Useful for tests and external anchoring.
func (l *Logger) LastHash() string {
	if l == nil {
		return ""
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.lastHash
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

// Verify scans an audit log stream and confirms chain integrity. Returns the
// number of records verified and the final Hash of the chain. Returns an
// error at the first chain violation (prev_hash mismatch or hash mismatch).
//
// Use Verify in tests, on operator request, and during periodic audits.
func Verify(r io.Reader) (count int, lastHash string, err error) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)

	expectedPrev := ""
	for sc.Scan() {
		line := sc.Bytes()
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}

		var rec Record
		if err := json.Unmarshal(line, &rec); err != nil {
			return count, lastHash, fmt.Errorf("record %d: decode: %w", count, err)
		}
		if rec.PrevHash != expectedPrev {
			return count, lastHash, fmt.Errorf(
				"record %d (%q): prev_hash mismatch: got %q, want %q",
				count, rec.Event, rec.PrevHash, expectedPrev,
			)
		}
		expected, err := computeHash(rec)
		if err != nil {
			return count, lastHash, fmt.Errorf("record %d: compute hash: %w", count, err)
		}
		if rec.Hash != expected {
			return count, lastHash, fmt.Errorf(
				"record %d (%q): hash mismatch: got %q, want %q",
				count, rec.Event, rec.Hash, expected,
			)
		}
		expectedPrev = rec.Hash
		lastHash = rec.Hash
		count++
	}
	if err := sc.Err(); err != nil {
		return count, lastHash, fmt.Errorf("scan: %w", err)
	}
	return count, lastHash, nil
}

// ErrNoEvent is returned by Record when r.Event is empty.
var ErrNoEvent = errors.New("auditlog: record missing event name")

// computeHash returns the SHA-256 hash (hex) of the canonical serialization
// of r with r.Hash treated as empty. PrevHash is part of the canonical form,
// chaining each record to its predecessor.
func computeHash(r Record) (string, error) {
	r.Hash = ""
	buf, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:]), nil
}

// readLastHash returns the Hash of the last record in path, or "" if the file
// is empty or does not exist. Reads the entire file; suitable for early
// phases where log size is bounded.
func readLastHash(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	if len(data) == 0 {
		return "", nil
	}

	lines := bytes.Split(bytes.TrimRight(data, "\n"), []byte{'\n'})
	if len(lines) == 0 {
		return "", nil
	}
	lastLine := lines[len(lines)-1]
	if len(bytes.TrimSpace(lastLine)) == 0 {
		return "", nil
	}

	var r Record
	if err := json.Unmarshal(lastLine, &r); err != nil {
		return "", fmt.Errorf("corrupt last record: %w", err)
	}
	return r.Hash, nil
}
