package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiterAllowsUpToBudget(t *testing.T) {
	rl := newRateLimiter(5) // 5 rpm
	for i := 0; i < 5; i++ {
		ok, _ := rl.allow("1.2.3.4")
		if !ok {
			t.Fatalf("request %d unexpectedly rate-limited", i+1)
		}
	}
	ok, retry := rl.allow("1.2.3.4")
	if ok {
		t.Fatal("6th request should have been limited")
	}
	if retry == "" {
		t.Fatal("expected Retry-After value")
	}
}

func TestRateLimiterPerIP(t *testing.T) {
	rl := newRateLimiter(2)
	for i := 0; i < 2; i++ {
		_, _ = rl.allow("1.1.1.1")
	}
	// 1.1.1.1 is exhausted; 2.2.2.2 should still pass.
	if ok, _ := rl.allow("2.2.2.2"); !ok {
		t.Fatal("different IP should not share bucket")
	}
}

func TestRateLimiterMiddleware(t *testing.T) {
	rl := newRateLimiter(2)
	called := 0
	h := rl.middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called++
		w.WriteHeader(200)
	}))

	doReq := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/api/x", nil)
		req.RemoteAddr = "9.9.9.9:1234"
		h.ServeHTTP(rec, req)
		return rec
	}

	for i := 0; i < 2; i++ {
		if rec := doReq(); rec.Code != 200 {
			t.Fatalf("req %d code=%d", i, rec.Code)
		}
	}
	rec := doReq()
	if rec.Code != 429 {
		t.Fatalf("expected 429, got %d", rec.Code)
	}
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("missing Retry-After header")
	}
	if called != 2 {
		t.Fatalf("handler called %d times, want 2", called)
	}
}

func TestRateLimiterRefills(t *testing.T) {
	rl := newRateLimiter(60) // 60 rpm = 1 per second
	if ok, _ := rl.allow("x"); !ok {
		t.Fatal("first should pass")
	}
	// Manually move the bucket back in time to simulate elapsed seconds.
	rl.mu.Lock()
	rl.buckets["x"].lastFill = time.Now().Add(-2 * time.Second)
	rl.buckets["x"].tokens = 0
	rl.mu.Unlock()
	if ok, _ := rl.allow("x"); !ok {
		t.Fatal("after 2s with 60rpm, should have refilled at least 1 token")
	}
}
