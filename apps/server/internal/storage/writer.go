package storage

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/sunny/sunny/apps/server/internal/bus"
	sdk "github.com/sunny/sunny/packages/sdk-go"
)

// WriterConfig tunes batching behavior.
type WriterConfig struct {
	BatchSize      int           // flush when buffer reaches this many records
	FlushInterval  time.Duration // flush after this long even if under BatchSize
	SubscribeQueue int           // bus subscription channel buffer
}

// DefaultWriterConfig is sized for a single-VPS deployment with a few hundred
// records/sec. Connectors that need more throughput should use ClickHouse mode
// once that ships in phase 2.5.
func DefaultWriterConfig() WriterConfig {
	return WriterConfig{
		BatchSize:      256,
		FlushInterval:  500 * time.Millisecond,
		SubscribeQueue: 1024,
	}
}

// Writer subscribes to a bus and batch-writes records to Storage.
//
// Each Writer runs one goroutine. Slow storage backpressures by filling the
// bus subscription channel — the bus drops records on overflow rather than
// blocking the publisher. The dropped count is exposed via Dropped().
type Writer struct {
	store    Storage
	bus      *bus.Bus
	cfg      WriterConfig
	logger   *slog.Logger
	written  atomic.Uint64
	flushes  atomic.Uint64
	failures atomic.Uint64
}

// NewWriter starts the writer. The bus subscription is established before
// this function returns, so records published after NewWriter returns are
// guaranteed to reach the writer (modulo subscription buffer overflow).
// Stop by cancelling ctx; ensure the caller waits on done before closing
// the storage.
func NewWriter(ctx context.Context, b *bus.Bus, store Storage, logger *slog.Logger, cfg WriterConfig) (w *Writer, done <-chan struct{}) {
	if cfg.BatchSize <= 0 {
		cfg = DefaultWriterConfig()
	}
	w = &Writer{
		store:  store,
		bus:    b,
		cfg:    cfg,
		logger: logger,
	}
	// Subscribe synchronously so callers don't race the goroutine startup.
	sub := b.Subscribe(nil, false)
	d := make(chan struct{})
	go w.run(ctx, sub, d)
	return w, d
}

func (w *Writer) Written() uint64  { return w.written.Load() }
func (w *Writer) Flushes() uint64  { return w.flushes.Load() }
func (w *Writer) Failures() uint64 { return w.failures.Load() }

func (w *Writer) run(ctx context.Context, sub *bus.Subscription, done chan<- struct{}) {
	defer close(done)
	defer sub.Close()

	buf := make([]sdk.Record, 0, w.cfg.BatchSize)
	ticker := time.NewTicker(w.cfg.FlushInterval)
	defer ticker.Stop()

	flush := func(reason string) {
		if len(buf) == 0 {
			return
		}
		// Use a fresh context with a small timeout so a hung DB doesn't pin
		// the writer if the parent ctx is still alive.
		fctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := w.store.Write(fctx, buf); err != nil {
			w.failures.Add(1)
			w.logger.Error("storage write failed", "reason", reason, "batch", len(buf), "err", err)
		} else {
			w.written.Add(uint64(len(buf)))
			w.flushes.Add(1)
		}
		buf = buf[:0]
	}

	for {
		select {
		case <-ctx.Done():
			flush("shutdown")
			return
		case <-ticker.C:
			flush("interval")
		case rec, ok := <-sub.C():
			if !ok {
				flush("subscription closed")
				return
			}
			buf = append(buf, rec)
			if len(buf) >= w.cfg.BatchSize {
				flush("batch full")
			}
		}
	}
}
