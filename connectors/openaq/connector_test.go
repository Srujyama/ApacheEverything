package openaq

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	sdk "github.com/sunny/sunny/packages/sdk-go"
)

type fakeCtx struct {
	mu      sync.Mutex
	records []sdk.Record
	cps     map[string]string
	secrets map[string]string
}

func newFake(secret string) *fakeCtx {
	return &fakeCtx{
		cps:     map[string]string{},
		secrets: map[string]string{SecretKey: secret},
	}
}

func (f *fakeCtx) Publish(_ context.Context, r sdk.Record) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records = append(f.records, r)
	return nil
}
func (f *fakeCtx) Logger() sdk.Logger        { return noopLogger{} }
func (f *fakeCtx) Secret(name string) string { return f.secrets[name] }
func (f *fakeCtx) Checkpoint(_ context.Context, k, v string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cps[k] = v
	return nil
}
func (f *fakeCtx) LoadCheckpoint(_ context.Context, k string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cps[k], nil
}

type noopLogger struct{}

func (noopLogger) Debug(string, ...any) {}
func (noopLogger) Info(string, ...any)  {}
func (noopLogger) Warn(string, ...any)  {}
func (noopLogger) Error(string, ...any) {}

type redirectTransport struct{ target string }

func (rt redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	u, _ := http.NewRequest(req.Method, rt.target, req.Body)
	u.Header = req.Header
	return http.DefaultTransport.RoundTrip(u)
}

func TestFixtureParses(t *testing.T) {
	body, err := os.ReadFile("testdata/measurements.json")
	if err != nil {
		t.Fatal(err)
	}
	var resp measurementsResp
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Results) != 3 {
		t.Fatalf("results = %d, want 3", len(resp.Results))
	}
	if resp.Results[0].Parameter.Name != "pm25" {
		t.Fatalf("first parameter = %q", resp.Results[0].Parameter.Name)
	}
}

func TestPollEmitsAllResults(t *testing.T) {
	body, err := os.ReadFile("testdata/measurements.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the API key header was forwarded.
		if r.Header.Get("X-API-Key") == "" {
			http.Error(w, "missing X-API-Key", http.StatusUnauthorized)
			return
		}
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := New().(*Connector)
	c.http.HTTP.Transport = redirectTransport{target: srv.URL}
	cfg := Config{CountriesID: 155}
	cfg.applyDefaults()

	rt := newFake("test-key")
	last := c.pollOnce(context.Background(), rt, cfg, "test-key", "")

	rt.mu.Lock()
	defer rt.mu.Unlock()
	if len(rt.records) != 3 {
		t.Fatalf("got %d records, want 3", len(rt.records))
	}
	for _, r := range rt.records {
		if r.Location == nil {
			t.Fatalf("record missing location: %+v", r)
		}
		if r.Tags["parameter"] == "" {
			t.Fatal("record missing parameter tag")
		}
	}
	if last == "" {
		t.Fatal("checkpoint not advanced")
	}
}

func TestPollDedupViaCheckpoint(t *testing.T) {
	body, err := os.ReadFile("testdata/measurements.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := New().(*Connector)
	c.http.HTTP.Transport = redirectTransport{target: srv.URL}
	cfg := Config{CountriesID: 155}
	cfg.applyDefaults()

	rt := newFake("test-key")
	last := c.pollOnce(context.Background(), rt, cfg, "test-key", "")
	first := len(rt.records)

	// Second poll with the checkpoint → no new records.
	c.pollOnce(context.Background(), rt, cfg, "test-key", last)
	if len(rt.records) != first {
		t.Fatalf("dedup failed: %d → %d", first, len(rt.records))
	}
}

func TestIdleWithoutKey(t *testing.T) {
	c := New()
	rt := newFake("") // empty secret
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx, rt, json.RawMessage(`{}`)) }()
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("got %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run didn't return")
	}
}
