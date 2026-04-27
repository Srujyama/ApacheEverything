package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sunny/sunny/apps/server/internal/bus"
	sdk "github.com/sunny/sunny/packages/sdk-go"
)

// quietLogger silences runtime logs during tests.
func quietLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeConnector lets tests script Run behavior.
type fakeConnector struct {
	id  string
	run func(ctx context.Context, rt sdk.Context) error
}

func (f *fakeConnector) Manifest() sdk.Manifest {
	return sdk.Manifest{ID: f.id, Name: f.id, Version: "test", Mode: sdk.ModePull, ConfigSchema: json.RawMessage(`{}`)}
}
func (f *fakeConnector) Validate(json.RawMessage) error { return nil }
func (f *fakeConnector) Run(ctx context.Context, rt sdk.Context, _ json.RawMessage) error {
	return f.run(ctx, rt)
}

// resetRegistry isolates registry state per test (registry is package-global).
func resetRegistry() {
	regMu.Lock()
	defer regMu.Unlock()
	reg = map[string]sdk.Connector{}
}

func TestRuntimePublishFlowsToBus(t *testing.T) {
	resetRegistry()
	Register(&fakeConnector{
		id: "fake",
		run: func(ctx context.Context, rt sdk.Context) error {
			payload, _ := json.Marshal(map[string]string{"hi": "there"})
			return rt.Publish(ctx, sdk.Record{Timestamp: time.Now(), Payload: payload})
		},
	})

	b := bus.New(8, 8)
	sub := b.Subscribe(nil, false)
	defer sub.Close()

	rt := NewRuntime(b, quietLogger(), EnvSecrets{}, nil)
	if err := rt.Start(context.Background(), InstanceSpec{InstanceID: "fake-1", Type: "fake"}); err != nil {
		t.Fatalf("start: %v", err)
	}

	select {
	case r := <-sub.C():
		if r.ConnectorID != "fake-1" {
			t.Fatalf("Publish should stamp instance id; got %q", r.ConnectorID)
		}
	case <-time.After(time.Second):
		t.Fatal("no record reached the bus")
	}
	rt.StopAll(time.Second)
}

func TestRuntimeRestartsOnCrash(t *testing.T) {
	resetRegistry()
	var calls atomic.Int32
	Register(&fakeConnector{
		id: "crashy",
		run: func(ctx context.Context, _ sdk.Context) error {
			n := calls.Add(1)
			if n < 3 {
				return errors.New("boom")
			}
			<-ctx.Done()
			return ctx.Err()
		},
	})
	rt := NewRuntime(bus.New(0, 8), quietLogger(), EnvSecrets{}, nil)
	if err := rt.Start(context.Background(), InstanceSpec{InstanceID: "crashy-1", Type: "crashy"}); err != nil {
		t.Fatalf("start: %v", err)
	}
	deadline := time.After(5 * time.Second)
	for {
		st, ok := rt.Status("crashy-1")
		if ok && st.State == StateRunning && calls.Load() >= 3 {
			break
		}
		select {
		case <-deadline:
			t.Fatalf("never recovered; calls=%d", calls.Load())
		case <-time.After(50 * time.Millisecond):
		}
	}
	rt.StopAll(time.Second)
}

func TestRuntimeRecoversFromPanic(t *testing.T) {
	resetRegistry()
	var calls atomic.Int32
	Register(&fakeConnector{
		id: "panicky",
		run: func(ctx context.Context, _ sdk.Context) error {
			n := calls.Add(1)
			if n == 1 {
				panic("oh no")
			}
			<-ctx.Done()
			return ctx.Err()
		},
	})
	rt := NewRuntime(bus.New(0, 8), quietLogger(), EnvSecrets{}, nil)
	if err := rt.Start(context.Background(), InstanceSpec{InstanceID: "panicky-1", Type: "panicky"}); err != nil {
		t.Fatalf("start: %v", err)
	}
	deadline := time.After(5 * time.Second)
	for calls.Load() < 2 {
		select {
		case <-deadline:
			t.Fatalf("did not recover from panic; calls=%d", calls.Load())
		case <-time.After(50 * time.Millisecond):
		}
	}
	st, _ := rt.Status("panicky-1")
	if st.LastError == "" {
		t.Fatal("expected LastError to be set after panic")
	}
	rt.StopAll(time.Second)
}

func TestRuntimeUnknownType(t *testing.T) {
	resetRegistry()
	rt := NewRuntime(bus.New(0, 8), quietLogger(), EnvSecrets{}, nil)
	err := rt.Start(context.Background(), InstanceSpec{InstanceID: "x", Type: "missing"})
	if err == nil {
		t.Fatal("expected error for unregistered type")
	}
}
