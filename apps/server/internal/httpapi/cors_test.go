package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseCORSEmpty(t *testing.T) {
	if c := parseCORS(""); c != nil {
		t.Fatalf("empty input should disable CORS, got %+v", c)
	}
	if c := parseCORS("   "); c != nil {
		t.Fatal("whitespace should disable CORS")
	}
}

func TestParseCORSWildcard(t *testing.T) {
	c := parseCORS("*")
	if c == nil || !c.allowAll {
		t.Fatalf("'*' should set allowAll, got %+v", c)
	}
}

func TestParseCORSExactList(t *testing.T) {
	c := parseCORS("https://a.example.com, https://b.example.com")
	if c == nil || c.allowAll || len(c.origins) != 2 {
		t.Fatalf("expected 2 origins, got %+v", c)
	}
	if !c.allowed("https://a.example.com") {
		t.Fatal("a.example.com not allowed")
	}
	if c.allowed("https://other.example.com") {
		t.Fatal("unlisted origin allowed")
	}
}

func TestCORSMiddlewareAttachesHeaders(t *testing.T) {
	c := parseCORS("https://app.example.com")
	h := c.middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/x", nil)
	req.Header.Set("Origin", "https://app.example.com")
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://app.example.com" {
		t.Fatalf("missing CORS header: %v", rec.Header())
	}
	if rec.Header().Get("Vary") != "Origin" {
		t.Fatal("missing Vary: Origin")
	}
	if rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatal("missing credentials header")
	}
}

func TestCORSMiddlewareIgnoresUnlisted(t *testing.T) {
	c := parseCORS("https://app.example.com")
	h := c.middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/x", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	h.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("unlisted origin got CORS header: %v", rec.Header())
	}
}

func TestCORSPreflight(t *testing.T) {
	c := parseCORS("*")
	h := c.middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Fatal("inner handler should not run for preflight")
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/api/x", nil)
	req.Header.Set("Origin", "https://anything.example")
	req.Header.Set("Access-Control-Request-Headers", "X-Custom-Header")
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight code = %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "X-Custom-Header" {
		t.Fatalf("allow-headers = %q", got)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "https://anything.example" {
		t.Fatal("preflight didn't echo origin")
	}
}
