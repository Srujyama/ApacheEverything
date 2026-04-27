package usgsearthquakes

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

// fakeContext implements sdk.Context for tests.
type fakeContext struct {
	mu          sync.Mutex
	records     []sdk.Record
	checkpoints map[string]string
}

func newFakeContext() *fakeContext { return &fakeContext{checkpoints: map[string]string{}} }

func (f *fakeContext) Publish(_ context.Context, r sdk.Record) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records = append(f.records, r)
	return nil
}
func (f *fakeContext) Logger() sdk.Logger        { return noopLogger{} }
func (f *fakeContext) Secret(string) string      { return "" }
func (f *fakeContext) Checkpoint(_ context.Context, k, v string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checkpoints[k] = v
	return nil
}
func (f *fakeContext) LoadCheckpoint(_ context.Context, k string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.checkpoints[k], nil
}

type noopLogger struct{}

func (noopLogger) Debug(string, ...any) {}
func (noopLogger) Info(string, ...any)  {}
func (noopLogger) Warn(string, ...any)  {}
func (noopLogger) Error(string, ...any) {}

// TestFixtureParses verifies the captured USGS feed decodes cleanly into
// our struct shape. Catches drift between USGS and our parser.
func TestFixtureParses(t *testing.T) {
	body, err := os.ReadFile("testdata/all_hour.json")
	if err != nil {
		t.Fatal(err)
	}
	var fc featureCollection
	if err := json.Unmarshal(body, &fc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(fc.Features) == 0 {
		t.Skip("fixture is empty (rare quake-free hour)")
	}
	for _, f := range fc.Features {
		if f.ID == "" {
			t.Fatal("missing event id")
		}
		if len(f.Geometry.Coordinates) < 2 {
			t.Fatalf("event %s has no coordinates", f.ID)
		}
		if f.Properties.Time == 0 {
			t.Fatalf("event %s has zero time", f.ID)
		}
	}
}

// TestPollAgainstFakeServer hits a synthetic HTTP server returning the
// captured fixture and verifies poll() emits the expected number of records.
func TestPollAgainstFakeServer(t *testing.T) {
	body, err := os.ReadFile("testdata/all_hour.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/geo+json")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := New().(*Connector)
	cfg := Config{Feed: "all_hour", PollSeconds: 60}
	cfg.applyDefaults()

	// Override the URL by injecting via a custom config struct path: the
	// helper builds the URL from cfg.Feed, so point that at our test server.
	// Easiest path is to call poll() with a temporary URL — but feedURL is
	// hardcoded against the USGS host. So we monkey-patch by overwriting
	// the http client to redirect any request to the test server.
	origDo := c.http.HTTP.Transport
	c.http.HTTP.Transport = redirectTransport{target: srv.URL}
	defer func() { c.http.HTTP.Transport = origDo }()

	rt := newFakeContext()
	if _, err := c.poll(context.Background(), rt, cfg, 0); err != nil {
		t.Fatalf("poll: %v", err)
	}

	rt.mu.Lock()
	defer rt.mu.Unlock()
	var fc featureCollection
	_ = json.Unmarshal(body, &fc)
	if len(rt.records) != len(fc.Features) {
		t.Fatalf("emitted %d records, want %d", len(rt.records), len(fc.Features))
	}
	if len(rt.records) > 0 {
		first := rt.records[0]
		if first.Location == nil {
			t.Fatal("first record missing location")
		}
		if first.SourceID == "" {
			t.Fatal("first record missing source ID")
		}
		// severity tag should always be set by magnitudeSeverity.
		if first.Tags["severity"] == "" {
			t.Fatal("first record missing severity tag")
		}
	}
}

// TestDedupViaCheckpoint verifies that polling twice with the same data
// only emits each event once.
func TestDedupViaCheckpoint(t *testing.T) {
	body, err := os.ReadFile("testdata/all_hour.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	c := New().(*Connector)
	cfg := Config{Feed: "all_hour", PollSeconds: 60}
	cfg.applyDefaults()
	c.http.HTTP.Transport = redirectTransport{target: srv.URL}

	rt := newFakeContext()
	last, err := c.poll(context.Background(), rt, cfg, 0)
	if err != nil {
		t.Fatal(err)
	}
	first := len(rt.records)
	if first == 0 {
		t.Skip("fixture empty")
	}
	// Second poll with the checkpoint should emit nothing.
	if _, err := c.poll(context.Background(), rt, cfg, last); err != nil {
		t.Fatal(err)
	}
	if len(rt.records) != first {
		t.Fatalf("dedup failed: %d → %d", first, len(rt.records))
	}
}

func TestMagnitudeSeverity(t *testing.T) {
	cases := []struct {
		mag float64
		out string
	}{{0, "info"}, {2.9, "info"}, {3.0, "warning"}, {4.4, "warning"}, {4.5, "critical"}, {5.9, "critical"}, {6.0, "emergency"}, {7.5, "emergency"}}
	for _, c := range cases {
		if got := magnitudeSeverity(c.mag); got != c.out {
			t.Fatalf("M%.1f → %q, want %q", c.mag, got, c.out)
		}
	}
}

// redirectTransport rewrites all outgoing requests to the test server's host.
type redirectTransport struct{ target string }

func (rt redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	u, _ := http.NewRequest(req.Method, rt.target+req.URL.Path, req.Body)
	u.Header = req.Header
	return http.DefaultTransport.RoundTrip(u)
}

// keep time import used
var _ = time.Now
