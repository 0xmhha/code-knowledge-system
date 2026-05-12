// Package footprint emits structured performance and decision events to a
// JSON Lines log so that cks operations (ckg queries, ckv embeddings,
// composer fusion, agent decisions) can be analyzed end-to-end via
// trace_id/run_id correlation.
//
// Volume control: footprint preserves ALL events at the configured Level.
// To reduce volume, raise Level (e.g. LevelInfo -> LevelWarn) rather than
// dropping events probabilistically. Sampling is intentionally NOT supported
// because cks is a single-process debug/perf tool: any dropped event
// destroys an end-to-end trace and defeats the package's purpose. For high-
// volume distributed services where sampling makes sense, use a different
// logger.
//
// For tamper-evident security records (capability denials, policy hits),
// use package auditlog instead. The two are designed to complement each
// other; see internal/observe.Audited for a helper that records to both in
// lock-step.
package footprint

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/0xmhha/code-knowledge-system/internal/envelope"
)

// Mode controls encoder/timestamp/caller configuration for the underlying zap
// logger.
//
//   - ModeProd: JSON encoder, RFC3339Nano timestamps, no caller info.
//     Suited for shipping to log aggregators.
//   - ModeDev:  Console encoder, colorized, with short caller. Suited for
//     interactive development.
type Mode string

const (
	ModeProd Mode = "prod"
	ModeDev  Mode = "dev"
)

// Level mirrors zapcore.Level as a string for config parsing.
type Level string

const (
	LevelDebug Level = "debug"
	LevelInfo  Level = "info"
	LevelWarn  Level = "warn"
	LevelError Level = "error"
)

// Config controls Logger construction.
type Config struct {
	// Writer receives serialized records. Defaults to os.Stdout when nil.
	Writer io.Writer
	// Mode selects encoder/format. Defaults to ModeProd when "".
	Mode Mode
	// Level is the minimum emitted severity. Defaults to LevelInfo.
	// Use this to tune log volume; debug/info events not at-or-above this
	// level are dropped wholesale (cheaper than emitting then filtering).
	Level Level
}

// Logger wraps a zap.Logger and auto-attaches envelope identifiers from
// context.
type Logger struct {
	zl     *zap.Logger
	closer io.Closer // optional, when Writer is owned by the logger
}

// Discard is a no-op logger used as a safe default for tests or when logging
// is disabled.
var Discard = &Logger{zl: zap.NewNop()}

// New constructs a Logger from cfg.
func New(cfg Config) (*Logger, error) {
	w := cfg.Writer
	if w == nil {
		w = os.Stdout
	}

	level, err := parseLevel(cfg.Level)
	if err != nil {
		return nil, err
	}

	mode := cfg.Mode
	if mode == "" {
		mode = ModeProd
	}

	var enc zapcore.Encoder
	switch mode {
	case ModeProd:
		ec := zap.NewProductionEncoderConfig()
		ec.TimeKey = "ts"
		ec.MessageKey = "event"
		ec.LevelKey = "level"
		ec.CallerKey = zapcore.OmitKey
		ec.StacktraceKey = zapcore.OmitKey
		ec.EncodeTime = zapcore.RFC3339NanoTimeEncoder
		enc = zapcore.NewJSONEncoder(ec)
	case ModeDev:
		ec := zap.NewDevelopmentEncoderConfig()
		ec.TimeKey = "ts"
		ec.MessageKey = "event"
		ec.EncodeTime = zapcore.RFC3339NanoTimeEncoder
		ec.EncodeLevel = zapcore.CapitalColorLevelEncoder
		enc = zapcore.NewConsoleEncoder(ec)
	default:
		return nil, fmt.Errorf("footprint: unknown mode %q", mode)
	}

	core := zapcore.NewCore(enc, zapcore.AddSync(w), level)
	zl := zap.New(core)
	return &Logger{zl: zl}, nil
}

func parseLevel(l Level) (zapcore.Level, error) {
	switch strings.ToLower(string(l)) {
	case "", "info":
		return zapcore.InfoLevel, nil
	case "debug":
		return zapcore.DebugLevel, nil
	case "warn", "warning":
		return zapcore.WarnLevel, nil
	case "error":
		return zapcore.ErrorLevel, nil
	default:
		return 0, fmt.Errorf("footprint: invalid level %q", l)
	}
}

// Event emits an event at info level with envelope identifiers attached.
// The event name forms the message field; additional structured fields are
// appended.
func (l *Logger) Event(ctx context.Context, name string, fields ...zap.Field) {
	if l == nil {
		return
	}
	l.zl.Info(name, l.envelopeFields(ctx, fields)...)
}

// Debug emits a debug-level event. Use for high-frequency sub-events
// (per-chunk embedding, per-citation scoring) that should be sample-able.
func (l *Logger) Debug(ctx context.Context, name string, fields ...zap.Field) {
	if l == nil {
		return
	}
	l.zl.Debug(name, l.envelopeFields(ctx, fields)...)
}

// Warn emits a warning-level event (recoverable degradation, e.g. ckv miss).
func (l *Logger) Warn(ctx context.Context, name string, fields ...zap.Field) {
	if l == nil {
		return
	}
	l.zl.Warn(name, l.envelopeFields(ctx, fields)...)
}

// Error emits an error-level event with an error field.
func (l *Logger) Error(ctx context.Context, name string, err error, fields ...zap.Field) {
	if l == nil {
		return
	}
	all := append([]zap.Field{zap.Error(err)}, fields...)
	l.zl.Error(name, l.envelopeFields(ctx, all)...)
}

func (l *Logger) envelopeFields(ctx context.Context, extra []zap.Field) []zap.Field {
	out := make([]zap.Field, 0, len(extra)+3)
	if id := envelope.TraceID(ctx); id != "" {
		out = append(out, zap.String("trace_id", id))
	}
	if id := envelope.RunID(ctx); id != "" {
		out = append(out, zap.String("run_id", id))
	}
	if envelope.DryRun(ctx) {
		out = append(out, zap.Bool("dry_run", true))
	}
	return append(out, extra...)
}

// Sync flushes any buffered records. Safe to call multiple times.
//
// On Darwin, zap returns ENOTTY/EINVAL when syncing os.Stdout/Stderr to a
// terminal; we tolerate those specifically because they do not indicate
// dropped records.
func (l *Logger) Sync() error {
	if l == nil || l.zl == nil {
		return nil
	}
	err := l.zl.Sync()
	if err == nil {
		return nil
	}
	msg := err.Error()
	if strings.Contains(msg, "inappropriate ioctl") || strings.Contains(msg, "invalid argument") {
		return nil
	}
	return err
}

// Close syncs and releases any owned writer.
func (l *Logger) Close() error {
	if l == nil {
		return nil
	}
	syncErr := l.Sync()
	if l.closer == nil {
		return syncErr
	}
	closeErr := l.closer.Close()
	return errors.Join(syncErr, closeErr)
}

// NewFile constructs a Logger that writes to path (appended). The file is
// closed when Logger.Close is invoked. Useful for per-run footprint files
// like logs/footprint/<run_id>.jsonl.
func NewFile(path string, cfg Config) (*Logger, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("footprint: open %q: %w", path, err)
	}
	cfg.Writer = f
	l, err := New(cfg)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	l.closer = f
	return l, nil
}
