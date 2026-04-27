package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	sdk "github.com/sunny/sunny/packages/sdk-go"
)

type fakeCtx struct {
	mu      sync.Mutex
	records []sdk.Record
	secret  string
}

func (f *fakeCtx) Publish(_ context.Context, r sdk.Record) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records = append(f.records, r)
	return nil
}
func (f *fakeCtx) Logger() sdk.Logger                                     { return noopLogger{} }
func (f *fakeCtx) Secret(string) string                                   { return f.secret }
func (f *fakeCtx) Checkpoint(context.Context, string, string) error       { return nil }
func (f *fakeCtx) LoadCheckpoint(context.Context, string) (string, error) { return "", nil }

type noopLogger struct{}

func (noopLogger) Debug(string, ...any) {}
func (noopLogger) Info(string, ...any)  {}
func (noopLogger) Warn(string, ...any)  {}
func (noopLogger) Error(string, ...any) {}

func newServer(t *testing.T, cfg Config, secret string) (*httptest.Server, *fakeCtx) {
	t.Helper()
	rt := &fakeCtx{secret: secret}
	c := New().(*Connector)
	cfgRaw, _ := json.Marshal(cfg)
	h, err := c.BuildPushHandler(rt, cfgRaw)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv, rt
}

func TestRejectsGet(t *testing.T) {
	srv, _ := newServer(t, Config{}, "")
	res, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("got %d", res.StatusCode)
	}
}

func TestPostWithoutTokenWhenRequired(t *testing.T) {
	srv, _ := newServer(t, Config{RequireToken: "abc"}, "")
	res, err := http.Post(srv.URL, "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("got %d", res.StatusCode)
	}
}

func TestPostWithSecretToken(t *testing.T) {
	srv, rt := newServer(t, Config{}, "secret-from-env")
	req, _ := http.NewRequest("POST", srv.URL, strings.NewReader(`{"x":1}`))
	req.Header.Set("X-Sunny-Token", "secret-from-env")
	req.Header.Set("X-Sunny-Source-Id", "src1")
	req.Header.Set("X-Sunny-Tag-Severity", "warning")
	req.Header.Set("X-Sunny-Lat", "37.0")
	req.Header.Set("X-Sunny-Lng", "-122.0")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("got %d body=%s", res.StatusCode, string(body))
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if len(rt.records) != 1 {
		t.Fatalf("records=%d", len(rt.records))
	}
	rec := rt.records[0]
	if rec.SourceID != "src1" {
		t.Fatalf("source id=%q", rec.SourceID)
	}
	if rec.Tags["severity"] != "warning" {
		t.Fatalf("severity tag=%v", rec.Tags)
	}
	if rec.Location == nil || rec.Location.Lat != 37.0 {
		t.Fatalf("location=%v", rec.Location)
	}
	var p map[string]int
	if err := json.Unmarshal(rec.Payload, &p); err != nil || p["x"] != 1 {
		t.Fatalf("payload=%s", rec.Payload)
	}
}

func TestNonJSONBodyWrapped(t *testing.T) {
	srv, rt := newServer(t, Config{}, "")
	res, err := http.Post(srv.URL, "text/plain", strings.NewReader("not json"))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("got %d", res.StatusCode)
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	var p map[string]string
	_ = json.Unmarshal(rt.records[0].Payload, &p)
	if p["raw"] != "not json" {
		t.Fatalf("payload=%v", p)
	}
}
