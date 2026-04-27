package nasafirms

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

func newFake() *fakeCtx { return &fakeCtx{cps: map[string]string{}, secrets: map[string]string{}} }

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

func TestPollParsesFixture(t *testing.T) {
	body, err := os.ReadFile("testdata/sample.csv")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/csv")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := New().(*Connector)
	c.http.HTTP.Transport = redirectTransport{target: srv.URL}

	cfg := Config{}
	cfg.applyDefaults()
	rt := newFake()
	got := c.pollOnce(context.Background(), rt, cfg, "test-key", "")

	rt.mu.Lock()
	defer rt.mu.Unlock()
	if len(rt.records) != 3 {
		t.Fatalf("emitted %d records, want 3", len(rt.records))
	}
	for _, r := range rt.records {
		if r.Location == nil {
			t.Fatal("missing location")
		}
		if r.Tags["satellite"] == "" {
			t.Fatal("missing satellite tag")
		}
		if r.SourceID == "" {
			t.Fatal("missing source id")
		}
	}
	// Checkpoint advanced.
	if got == "" {
		t.Fatal("checkpoint not advanced")
	}
}

func TestParseFIRMSTime(t *testing.T) {
	cases := []struct {
		date, atime string
		wantHour    int
	}{
		{"2026-04-26", "1830", 18},
		{"2026-04-26", "0830", 8},
		{"2026-04-26", "830", 8}, // 3-char form normalized
	}
	for _, tc := range cases {
		got := parseFIRMSTime(tc.date, tc.atime)
		if got.Hour() != tc.wantHour {
			t.Fatalf("parseFIRMSTime(%q, %q) hour = %d, want %d", tc.date, tc.atime, got.Hour(), tc.wantHour)
		}
	}
	// Empty / malformed → falls back to "now".
	now := time.Now()
	got := parseFIRMSTime("", "")
	if got.Before(now.Add(-time.Second)) || got.After(now.Add(time.Second)) {
		t.Fatalf("malformed time should fall back to ~now, got %v", got)
	}
}

func TestIdleWithoutKey(t *testing.T) {
	// When the secret is missing, Run should idle (block on ctx.Done).
	c := New()
	rt := newFake()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx, rt, json.RawMessage(`{}`)) }()
	cancel()
	select {
	case err := <-done:
		if err != context.Canceled {
			t.Fatalf("idle return = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run didn't return after cancel")
	}
}
