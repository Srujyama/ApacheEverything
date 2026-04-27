package alerts

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/sunny/sunny/apps/server/internal/bus"
	"github.com/sunny/sunny/apps/server/internal/storage"
	sdk "github.com/sunny/sunny/packages/sdk-go"
)

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func newStore(t *testing.T) storage.Storage {
	t.Helper()
	s, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestEngineFiresOnSeverity(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	if err := store.SaveRule(ctx, storage.AlertRule{
		ID: "r1", Name: "criticals", Enabled: true,
		SeverityIn: []string{"critical"},
	}); err != nil {
		t.Fatal(err)
	}

	b := bus.New(0, 64)
	e := New(b, store, quietLogger())

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = e.Run(runCtx) }()
	time.Sleep(50 * time.Millisecond) // let it subscribe

	b.Publish(ctx, sdk.Record{
		Timestamp: time.Now(), ConnectorID: "earthquakes",
		SourceID: "q1", Tags: map[string]string{"severity": "critical"},
		Payload: json.RawMessage(`{"place":"M5 near Berkeley"}`),
	})
	b.Publish(ctx, sdk.Record{
		Timestamp: time.Now(), ConnectorID: "earthquakes",
		SourceID: "q2", Tags: map[string]string{"severity": "info"},
	})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		alerts, _ := store.ListAlerts(ctx, 10)
		if len(alerts) >= 1 {
			if alerts[0].Severity != "critical" {
				t.Fatalf("got severity %q", alerts[0].Severity)
			}
			if alerts[0].Headline != "M5 near Berkeley" {
				t.Fatalf("got headline %q", alerts[0].Headline)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("timeout waiting for alert")
}

func TestEngineDedupe(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	_ = store.SaveRule(ctx, storage.AlertRule{
		ID: "r1", Name: "criticals", Enabled: true,
		SeverityIn: []string{"critical"},
	})

	b := bus.New(0, 64)
	e := New(b, store, quietLogger())
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() { _ = e.Run(runCtx) }()
	time.Sleep(50 * time.Millisecond)

	for i := 0; i < 5; i++ {
		b.Publish(ctx, sdk.Record{
			Timestamp: time.Now(), ConnectorID: "earthquakes",
			SourceID: "same-quake",
			Tags:     map[string]string{"severity": "critical"},
		})
	}
	time.Sleep(300 * time.Millisecond)
	alerts, _ := store.ListAlerts(ctx, 10)
	if len(alerts) != 1 {
		t.Fatalf("expected 1 alert (dedupe), got %d", len(alerts))
	}
}

func TestSeedDefaultRule(t *testing.T) {
	store := newStore(t)
	ctx := context.Background()
	e := New(bus.New(0, 8), store, quietLogger())
	if err := e.SeedDefaultRule(ctx); err != nil {
		t.Fatal(err)
	}
	rs, _ := store.ListRules(ctx)
	if len(rs) != 1 || rs[0].ID != "default-critical" {
		t.Fatalf("seed rule wrong: %+v", rs)
	}
	// Idempotent: second call doesn't add another.
	if err := e.SeedDefaultRule(ctx); err != nil {
		t.Fatal(err)
	}
	rs, _ = store.ListRules(ctx)
	if len(rs) != 1 {
		t.Fatalf("seeded twice: %d rules", len(rs))
	}
}
