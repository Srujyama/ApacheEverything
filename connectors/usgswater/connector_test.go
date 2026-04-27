package usgswater

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
	body, err := os.ReadFile("testdata/iv.json")
	if err != nil {
		t.Fatal(err)
	}
	var resp nwisResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Value.TimeSeries) == 0 {
		t.Skip("fixture has no time series")
	}
	first := resp.Value.TimeSeries[0]
	if len(first.SourceInfo.SiteCode) == 0 {
		t.Fatal("site code missing")
	}
	if first.SourceInfo.GeoLocation.Geog.Lat == 0 {
		t.Fatal("lat missing")
	}
}

type redirectTransport struct{ target string }

func (rt redirectTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	u, _ := http.NewRequest(req.Method, rt.target, req.Body)
	u.Header = req.Header
	return http.DefaultTransport.RoundTrip(u)
}

func TestPollEmits(t *testing.T) {
	body, err := os.ReadFile("testdata/iv.json")
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
	c.pollOnce(context.Background(), rt, cfg, "")

	rt.mu.Lock()
	defer rt.mu.Unlock()
	if len(rt.records) == 0 {
		t.Skip("fixture had no values")
	}
	for _, r := range rt.records {
		if r.Location == nil {
			t.Fatal("water record missing location")
		}
		if r.Tags["site"] == "" {
			t.Fatal("water record missing site tag")
		}
		if r.SourceID == "" {
			t.Fatal("water record missing source id")
		}
	}
}
