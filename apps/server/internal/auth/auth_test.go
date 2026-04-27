package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestDisabledWhenNoHash(t *testing.T) {
	m, err := NewManager("", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if m.Enabled() {
		t.Fatal("expected disabled")
	}
	// Middleware is a pass-through.
	called := false
	h := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/foo", nil))
	if !called || rec.Code != 200 {
		t.Fatalf("middleware should pass through; called=%v code=%d", called, rec.Code)
	}
}

func TestLoginAndCookie(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("hunter2"), bcrypt.MinCost)
	m, err := NewManager(string(hash), "test-key-aaaaaaaaaaaaaaaaaaaaaa", "")
	if err != nil {
		t.Fatal(err)
	}
	if !m.Enabled() {
		t.Fatal("expected enabled")
	}

	// Bad password → 401.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/auth/login",
		strings.NewReader(`{"password":"wrong"}`))
	m.LoginHandler(rec, req)
	if rec.Code != 401 {
		t.Fatalf("bad pw code = %d", rec.Code)
	}

	// Good password → 200 + cookie.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/auth/login",
		strings.NewReader(`{"password":"hunter2"}`))
	m.LoginHandler(rec, req)
	if rec.Code != 200 {
		t.Fatalf("good pw code = %d", rec.Code)
	}
	cookie := rec.Result().Cookies()
	if len(cookie) == 0 || cookie[0].Name != CookieName {
		t.Fatal("expected session cookie")
	}

	// Cookie passes middleware.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/api/protected", nil)
	req.AddCookie(cookie[0])
	called := false
	h := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
	}))
	h.ServeHTTP(rec, req)
	if !called {
		t.Fatal("middleware blocked valid cookie")
	}

	// Missing cookie → 401.
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/api/protected", nil))
	if rec.Code != 401 {
		t.Fatalf("missing cookie code = %d", rec.Code)
	}
}

func TestRejectInvalidHashFormat(t *testing.T) {
	if _, err := NewManager("not-a-bcrypt-hash", "", ""); err == nil {
		t.Fatal("expected error for non-bcrypt hash")
	}
}

func TestExpiredToken(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("p"), bcrypt.MinCost)
	m, _ := NewManager(string(hash), "k123456789012345678901234567890123", "")
	tok, err := m.Issue(-time.Hour) // already expired
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Validate(tok); err == nil {
		t.Fatal("expected validation error for expired token")
	}
}

func TestTokenAuth(t *testing.T) {
	m, err := NewManager("", "", "tok-1234567890123456,tok-abcdefghijklmnop")
	if err != nil {
		t.Fatal(err)
	}
	if !m.Enabled() || m.PasswordEnabled() {
		t.Fatalf("token-only flags wrong: enabled=%v passwordEnabled=%v", m.Enabled(), m.PasswordEnabled())
	}
	called := false
	h := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
	}))
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/x", nil)
	req.Header.Set("Authorization", "Bearer tok-1234567890123456")
	h.ServeHTTP(rec, req)
	if !called {
		t.Fatal("valid token rejected")
	}

	// Wrong token.
	rec = httptest.NewRecorder()
	called = false
	req2 := httptest.NewRequest("GET", "/api/x", nil)
	req2.Header.Set("Authorization", "Bearer wrong")
	h.ServeHTTP(rec, req2)
	if called || rec.Code != 401 {
		t.Fatalf("wrong token allowed; called=%v code=%d", called, rec.Code)
	}

	// Missing header.
	rec = httptest.NewRecorder()
	called = false
	req3 := httptest.NewRequest("GET", "/api/x", nil)
	h.ServeHTTP(rec, req3)
	if called || rec.Code != 401 {
		t.Fatalf("missing header allowed; called=%v code=%d", called, rec.Code)
	}
}

func TestRejectsShortTokens(t *testing.T) {
	if _, err := NewManager("", "", "short"); err == nil {
		t.Fatal("expected error for short token")
	}
}

func TestCookieAndTokenCoexist(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("p"), bcrypt.MinCost)
	m, err := NewManager(string(hash), "k123456789012345678901234567890123", "tok-1234567890123456")
	if err != nil {
		t.Fatal(err)
	}
	called := false
	h := m.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
	}))
	// Token works.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/x", nil)
	req.Header.Set("Authorization", "Bearer tok-1234567890123456")
	h.ServeHTTP(rec, req)
	if !called {
		t.Fatal("token failed when both factors are configured")
	}

	// Cookie works.
	tok, _ := m.Issue(time.Hour)
	rec = httptest.NewRecorder()
	called = false
	req2 := httptest.NewRequest("GET", "/api/x", nil)
	req2.AddCookie(&http.Cookie{Name: CookieName, Value: tok})
	h.ServeHTTP(rec, req2)
	if !called {
		t.Fatal("cookie failed when both factors are configured")
	}
}

func TestLoginRejectsCrossOriginPOST(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("p"), bcrypt.MinCost)
	m, _ := NewManager(string(hash), "k123456789012345678901234567890123", "")

	// Cross-origin: Origin header doesn't match request Host.
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/auth/login",
		strings.NewReader(`{"password":"p"}`))
	req.Host = "sunny.local"
	req.Header.Set("Origin", "https://evil.example.com")
	m.LoginHandler(rec, req)
	if rec.Code != 403 {
		t.Fatalf("cross-origin login got %d, want 403", rec.Code)
	}

	// Same-origin: Origin matches Host.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/auth/login",
		strings.NewReader(`{"password":"p"}`))
	req.Host = "sunny.local"
	req.Header.Set("Origin", "https://sunny.local")
	m.LoginHandler(rec, req)
	if rec.Code != 200 {
		t.Fatalf("same-origin login got %d, want 200", rec.Code)
	}

	// No Origin / Referer (curl-style client): allowed.
	rec = httptest.NewRecorder()
	req = httptest.NewRequest("POST", "/api/auth/login",
		strings.NewReader(`{"password":"p"}`))
	req.Host = "sunny.local"
	m.LoginHandler(rec, req)
	if rec.Code != 200 {
		t.Fatalf("no-origin login got %d, want 200", rec.Code)
	}
}
