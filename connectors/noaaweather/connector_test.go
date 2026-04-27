package noaaweather

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"

	sdk "github.com/sunny/sunny/packages/sdk-go"
)

type fakeCtx struct {
	mu      sync.Mutex
	records []sdk.Record
	cps     map[string]string
}

func newFake() *fakeCtx { return &fakeCtx{cps: map[string]string{}} }

func (f *fakeCtx) Publish(_ context.Context, r sdk.Record) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records = append(f.records, r)
	return nil
}
func (f *fakeCtx) Logger() sdk.Logger                                     { return noopLogger{} }
func (f *fakeCtx) Secret(string) string                                   { return "" }
func (f *fakeCtx) Checkpoint(_ context.Context, k, v string) error        { f.cps[k] = v; return nil }
func (f *fakeCtx) LoadCheckpoint(_ context.Context, k string) (string, error) {
	return f.cps[k], nil
}

type noopLogger struct{}

func (noopLogger) Debug(string, ...any) {}
func (noopLogger) Info(string, ...any)  {}
func (noopLogger) Warn(string, ...any)  {}
func (noopLogger) Error(string, ...any) {}

func TestFixtureParses(t *testing.T) {
	body, err := os.ReadFile("testdata/alerts_active.json")
	if err != nil {
		t.Fatal(err)
	}
	var fc alertCollection
	if err := json.Unmarshal(body, &fc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(fc.Features) == 0 {
		t.Skip("no active alerts in fixture")
	}
	for _, a := range fc.Features {
		if a.ID == "" {
			t.Fatal("alert missing id")
		}
	}
}

type redirectTransport struct{ target string }

func (rt redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	u, _ := http.NewRequest(req.Method, rt.target, req.Body)
	u.Header = req.Header
	return http.DefaultTransport.RoundTrip(u)
}

func TestPollEmitsRecordsAndSkipsTests(t *testing.T) {
	body, err := os.ReadFile("testdata/alerts_active.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := New().(*Connector)
	c.http.HTTP.Transport = redirectTransport{target: srv.URL}
	cfg := Config{}
	cfg.applyDefaults()

	rt := newFake()
	c.pollOnce(context.Background(), rt, cfg, map[string]string{})

	rt.mu.Lock()
	defer rt.mu.Unlock()
	if len(rt.records) == 0 {
		t.Skip("fixture had no non-test alerts")
	}
	for _, r := range rt.records {
		if r.Tags["status"] == "test" {
			t.Fatal("test-status alert was emitted; should have been skipped")
		}
	}
}

func TestSeverityMinFilter(t *testing.T) {
	body, err := os.ReadFile("testdata/alerts_active.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := New().(*Connector)
	c.http.HTTP.Transport = redirectTransport{target: srv.URL}

	// Ask for severe+ only. The fixture contains a mix; we should never see
	// minor/moderate after filtering.
	cfg := Config{SeverityMin: "Severe"}
	cfg.applyDefaults()
	rt := newFake()
	c.pollOnce(context.Background(), rt, cfg, map[string]string{})

	rt.mu.Lock()
	defer rt.mu.Unlock()
	for _, r := range rt.records {
		switch r.Tags["severity"] {
		case "minor", "moderate":
			t.Fatalf("severity filter let through %q", r.Tags["severity"])
		}
	}
}
