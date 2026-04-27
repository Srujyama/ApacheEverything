package storage

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/sunny/sunny/apps/server/internal/bus"
	sdk "github.com/sunny/sunny/packages/sdk-go"
)

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestWriterFlushesBatch(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	b := bus.New(0, 256)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := WriterConfig{BatchSize: 5, FlushInterval: 50 * time.Millisecond, SubscribeQueue: 256}
	w, done := NewWriter(ctx, b, store, quietLogger(), cfg)

	for i := 0; i < 12; i++ {
		payload, _ := json.Marshal(map[string]int{"i": i})
		b.Publish(ctx, sdk.Record{Timestamp: time.Now().UTC(), ConnectorID: "hello-1", Payload: payload})
	}
	// Wait long enough for at least 2 batches + 1 timer flush of leftovers.
	deadline := time.Now().Add(2 * time.Second)
	for w.Written() < 12 && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
	if w.Written() != 12 {
		t.Fatalf("written=%d, want 12 (flushes=%d, failures=%d)", w.Written(), w.Flushes(), w.Failures())
	}
	got, err := store.Recent(context.Background(), 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 12 {
		t.Fatalf("recent=%d, want 12", len(got))
	}
}
