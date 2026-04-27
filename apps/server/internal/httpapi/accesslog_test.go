package httpapi

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAccessLogIncludesRequestFields(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	h := accessLogMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(201)
		_, _ = w.Write([]byte("hi"))
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/connectors", nil)
	h.ServeHTTP(rec, req)

	var entry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &entry); err != nil {
		t.Fatalf("non-JSON log line: %s", buf.String())
	}
	if entry["msg"] != "http" {
		t.Fatalf("msg = %v", entry["msg"])
	}
	if entry["method"] != "POST" || entry["path"] != "/api/connectors" {
		t.Fatalf("missing fields: %+v", entry)
	}
	if entry["status"].(float64) != 201 {
		t.Fatalf("status = %v", entry["status"])
	}
	if entry["bytes"].(float64) != 2 {
		t.Fatalf("bytes = %v", entry["bytes"])
	}
}

func TestAccessLogSkipsHealth(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	h := accessLogMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	for _, path := range []string{"/api/health", "/api/version", "/assets/index.js"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", path, nil)
		h.ServeHTTP(rec, req)
	}
	if buf.Len() > 0 {
		t.Fatalf("expected no log for skipped paths, got: %s", buf.String())
	}
}

func TestAccessLogLevelFromStatus(t *testing.T) {
	cases := []struct {
		status   int
		wantLvl  string
	}{
		{200, "INFO"},
		{301, "INFO"},
		{404, "WARN"},
		{500, "ERROR"},
	}
	for _, tc := range cases {
		var buf bytes.Buffer
		logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
		h := accessLogMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
		}))
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/x", nil))
		if !strings.Contains(buf.String(), `"level":"`+tc.wantLvl+`"`) {
			t.Fatalf("status %d expected level %s, got: %s", tc.status, tc.wantLvl, buf.String())
		}
	}
}
