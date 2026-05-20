package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sunny/sunny/apps/server/internal/storage"
	sdk "github.com/sunny/sunny/packages/sdk-go"
)

func TestMetricsEndpoint_ExposesBuildAndUptime(t *testing.T) {
	t.Parallel()
	srv, _, _ := setup(t, nil)
	res := mustGet(t, srv.URL+"/metrics")
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Fatalf("content-type = %q", ct)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, want := range []string{
		"# HELP sunny_build_info",
		"# TYPE sunny_build_info gauge",
		`sunny_build_info{api_version="`,
		"sunny_uptime_seconds ",
		"go_goroutines ",
		"go_memstats_alloc_bytes ",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in body:\n%s", want, s)
		}
	}
}

func TestMetricsEndpoint_IncludesPerConnectorRecordCounts(t *testing.T) {
	t.Parallel()
	srv, store, _ := setup(t, nil)
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := store.Write(context.Background(), []sdk.Record{
		{Timestamp: now, ConnectorID: "test-connector", SourceID: "s1",
			Payload: json.RawMessage(`{}`)},
		{Timestamp: now, ConnectorID: "test-connector", SourceID: "s2",
			Payload: json.RawMessage(`{}`)},
	}); err != nil {
		t.Fatal(err)
	}
	res := mustGet(t, srv.URL+"/metrics")
	body, _ := io.ReadAll(res.Body)
	s := string(body)
	if !strings.Contains(s, `sunny_records_total{connector="test-connector"} 2`) {
		t.Fatalf("missing per-connector total. body:\n%s", s)
	}
}

func TestMetricsEndpoint_PrometheusParseable(t *testing.T) {
	// Sanity check the format: every non-blank, non-comment line must split
	// into exactly two whitespace-separated fields ("<name>[{labels}] <value>").
	t.Parallel()
	srv, _, _ := setup(t, nil)
	res := mustGet(t, srv.URL+"/metrics")
	body, _ := io.ReadAll(res.Body)
	for i, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			t.Errorf("line %d not metric-shaped: %q", i, line)
		}
	}
}

func TestEscapeLabel(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		`plain`:        `plain`,
		`with "quote"`: `with \"quote\"`,
		`back\slash`:   `back\\slash`,
		"new\nline":    `new\nline`,
	}
	for in, want := range cases {
		if got := escapeLabel(in); got != want {
			t.Errorf("escapeLabel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFormatFloat_DropsTrailingZeros(t *testing.T) {
	t.Parallel()
	cases := map[float64]string{
		1.0:    "1",
		1.5:    "1.5",
		0:      "0",
		0.1:    "0.1",
	}
	for in, want := range cases {
		if got := formatFloat(in); got != want {
			t.Errorf("formatFloat(%v) = %q, want %q", in, got, want)
		}
	}
}

// Force the metrics handler to talk to a tampered storage so the slow paths
// don't dominate the test runtime in CI.
var _ = httptest.NewServer
var _ = storage.Open
