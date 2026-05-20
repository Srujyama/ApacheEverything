package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDoctor_AllChecksPassAgainstFakeServer(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"status":"ok"}`)) })
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("/api/version", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"version":"0.1","apiVersion":1}`))
	})
	mux.HandleFunc("/api/v1/version", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"version":"0.1","apiVersion":1}`))
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`# HELP sunny_build_info x` + "\n" + `sunny_build_info{version="0.1"} 1` + "\n"))
	})
	mux.HandleFunc("/api/auth/status", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"enabled":false,"loggedIn":true}`))
	})
	mux.HandleFunc("/api/connectors/instances", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[]`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	if err := doctorCmd([]string{"--server", srv.URL}); err != nil {
		t.Fatalf("doctor returned %v", err)
	}
}

func TestDoctor_FailsOnMissingApiVersion(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	// Most endpoints OK, but /api/version omits apiVersion → that specific check fails.
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{"status":"ok"}`)) })
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	mux.HandleFunc("/api/version", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"version":"0.1"}`)) // no apiVersion
	})
	mux.HandleFunc("/api/v1/version", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"version":"0.1"}`))
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`sunny_build_info 1` + "\n"))
	})
	mux.HandleFunc("/api/auth/status", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`{}`)) })
	mux.HandleFunc("/api/connectors/instances", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(`[]`)) })
	srv := httptest.NewServer(mux)
	defer srv.Close()
	err := doctorCmd([]string{"--server", srv.URL})
	if err == nil {
		t.Fatal("expected failure when /api/version omits apiVersion")
	}
	if !strings.Contains(err.Error(), "failed") {
		t.Fatalf("error = %v", err)
	}
}
