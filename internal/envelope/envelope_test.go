package envelope

import (
	"context"
	"testing"
)

func TestTraceID_RoundTrip(t *testing.T) {
	t.Parallel()
	id := NewTraceID()
	if len(id) != 2*idBytes {
		t.Fatalf("NewTraceID length = %d, want %d", len(id), 2*idBytes)
	}
	ctx := WithTraceID(context.Background(), id)
	if got := TraceID(ctx); got != id {
		t.Fatalf("TraceID = %q, want %q", got, id)
	}
}

func TestRunID_RoundTrip(t *testing.T) {
	t.Parallel()
	id := NewRunID()
	if len(id) != 2*idBytes {
		t.Fatalf("NewRunID length = %d, want %d", len(id), 2*idBytes)
	}
	ctx := WithRunID(context.Background(), id)
	if got := RunID(ctx); got != id {
		t.Fatalf("RunID = %q, want %q", got, id)
	}
}

func TestDryRun_Default(t *testing.T) {
	t.Parallel()
	if DryRun(context.Background()) {
		t.Fatal("DryRun default = true, want false")
	}
}

func TestDryRun_Set(t *testing.T) {
	t.Parallel()
	ctx := WithDryRun(context.Background(), true)
	if !DryRun(ctx) {
		t.Fatal("DryRun after WithDryRun(true) = false, want true")
	}
	ctx2 := WithDryRun(ctx, false)
	if DryRun(ctx2) {
		t.Fatal("DryRun after WithDryRun(false) = true, want false")
	}
}

func TestEnsureTraceID(t *testing.T) {
	t.Parallel()
	ctx := EnsureTraceID(context.Background())
	if TraceID(ctx) == "" {
		t.Fatal("EnsureTraceID produced empty trace_id")
	}

	preset := "preset-id"
	ctx2 := EnsureTraceID(WithTraceID(context.Background(), preset))
	if TraceID(ctx2) != preset {
		t.Fatalf("EnsureTraceID overwrote existing id: got %q, want %q", TraceID(ctx2), preset)
	}
}

func TestEnsureRunID(t *testing.T) {
	t.Parallel()
	ctx := EnsureRunID(context.Background())
	if RunID(ctx) == "" {
		t.Fatal("EnsureRunID produced empty run_id")
	}
}

func TestIDs_AreUnique(t *testing.T) {
	t.Parallel()
	seen := make(map[string]struct{}, 1000)
	for range 1000 {
		id := NewTraceID()
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate trace_id collision: %q", id)
		}
		seen[id] = struct{}{}
	}
}
