// Package auth implements authentication for Sunny.
//
// Two factors gate access:
//   - SUNNY_PASSWORD_HASH (bcrypt) → enables the password login + cookie
//     session flow. Browser users hit /api/auth/login.
//   - SUNNY_API_TOKENS (comma-separated) → enables Bearer-token API auth.
//     Scripts and CLI tools send `Authorization: Bearer <token>`.
//
// If both are unset, auth is disabled and Middleware is a no-op (embedded
// mode). If either is set, Middleware accepts a valid cookie OR a valid
// token; the request just needs one of them.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// urlParse aliased so the reference in sameOrigin reads cleanly.
var urlParse = url.Parse

// CookieName is the auth session cookie.
const CookieName = "sunny_session"

// Manager holds the bcrypt password hash, HMAC cookie key, and API tokens.
type Manager struct {
	bcryptHash []byte
	hmacKey    []byte
	apiTokens  map[string]struct{} // set of accepted bearer tokens
}

// NewManager constructs a Manager.
//
//   - passwordHash: bcrypt hash for cookie/login flow. Empty disables it.
//   - hmacKey: cookie-signing key. Empty → random per-startup.
//   - apiTokens: comma-separated list of accepted Bearer tokens. Empty
//     disables token auth. Tokens are stored in a constant-time-lookup map.
//
// At least one factor (password or tokens) must be set for Enabled() to
// return true. If both are empty the manager is a no-op.
func NewManager(passwordHash, hmacKey, apiTokens string) (*Manager, error) {
	m := &Manager{apiTokens: map[string]struct{}{}}
	if h := strings.TrimSpace(passwordHash); h != "" {
		if !strings.HasPrefix(h, "$2a$") && !strings.HasPrefix(h, "$2b$") && !strings.HasPrefix(h, "$2y$") {
			return nil, errors.New("SUNNY_PASSWORD_HASH must be a bcrypt hash; use `sunny-cli hash-password` to generate one")
		}
		m.bcryptHash = []byte(h)
	}
	for _, tok := range strings.Split(apiTokens, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		if len(tok) < 16 {
			return nil, errors.New("SUNNY_API_TOKENS entries must be at least 16 characters")
		}
		m.apiTokens[tok] = struct{}{}
	}
	if k := strings.TrimSpace(hmacKey); k != "" {
		m.hmacKey = []byte(k)
	} else {
		m.hmacKey = make([]byte, 32)
		if _, err := rand.Read(m.hmacKey); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// Enabled reports whether the manager has any auth factor configured.
func (m *Manager) Enabled() bool {
	return len(m.bcryptHash) > 0 || len(m.apiTokens) > 0
}

// PasswordEnabled reports whether the cookie/login flow is active.
func (m *Manager) PasswordEnabled() bool { return len(m.bcryptHash) > 0 }

// hasValidToken constant-time-checks the Bearer header.
func (m *Manager) hasValidToken(authHeader string) bool {
	if authHeader == "" || len(m.apiTokens) == 0 {
		return false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		return false
	}
	got := strings.TrimSpace(authHeader[len(prefix):])
	if got == "" {
		return false
	}
	// Map lookup leaks size info on its own, but the timing risk on a small
	// admin set is acceptable. Walk the set with constant-time-compare to
	// avoid the obvious early-exit.
	gotB := []byte(got)
	ok := false
	for tok := range m.apiTokens {
		if hmac.Equal(gotB, []byte(tok)) {
			ok = true
		}
	}
	return ok
}

// Verify returns nil if the plaintext matches the configured bcrypt hash.
func (m *Manager) Verify(plaintext string) error {
	if !m.PasswordEnabled() {
		return errors.New("password auth not configured")
	}
	return bcrypt.CompareHashAndPassword(m.bcryptHash, []byte(plaintext))
}

// session encodes a small JSON blob plus an HMAC tag.
type session struct {
	Issued int64 `json:"iat"`
	Expiry int64 `json:"exp"`
}

// Issue returns a signed session string suitable for setting in a cookie.
func (m *Manager) Issue(ttl time.Duration) (string, error) {
	s := session{Issued: time.Now().Unix(), Expiry: time.Now().Add(ttl).Unix()}
	body, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(body)
	mac := hmac.New(sha256.New, m.hmacKey)
	mac.Write([]byte(encoded))
	tag := hex.EncodeToString(mac.Sum(nil))
	return encoded + "." + tag, nil
}

// Validate checks the cookie value's HMAC and expiry.
func (m *Manager) Validate(token string) error {
	parts := strings.SplitN(token, ".", 2)
	if len(parts) != 2 {
		return errors.New("malformed token")
	}
	mac := hmac.New(sha256.New, m.hmacKey)
	mac.Write([]byte(parts[0]))
	want := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(parts[1])) {
		return errors.New("bad signature")
	}
	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return err
	}
	var s session
	if err := json.Unmarshal(body, &s); err != nil {
		return err
	}
	if time.Now().Unix() > s.Expiry {
		return errors.New("expired")
	}
	return nil
}

// Middleware returns a chi-compatible middleware that requires either a
// valid session cookie OR a valid Bearer token. If neither auth factor is
// configured, the middleware is a pass-through.
func (m *Manager) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !m.Enabled() {
			next.ServeHTTP(w, r)
			return
		}
		// Token check (cheaper, cookie-less callers preferred for API).
		if m.hasValidToken(r.Header.Get("Authorization")) {
			next.ServeHTTP(w, r)
			return
		}
		// Cookie check for browser flow.
		if c, err := r.Cookie(CookieName); err == nil {
			if err := m.Validate(c.Value); err == nil {
				next.ServeHTTP(w, r)
				return
			}
		}
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	})
}

// SessionTTL is how long a successful login lasts.
const SessionTTL = 7 * 24 * time.Hour

// LoginHandler verifies the password and sets the cookie.
//
// POST /api/auth/login {"password": "..."}
// Returns 200 on success, 401 on failure, 204 if password auth is not enabled.
//
// CSRF: rejects requests where Origin (or Referer) is set to a different
// host than the request itself. Same-host POSTs (the normal SPA flow) and
// API tools that don't send Origin both pass.
func (m *Manager) LoginHandler(w http.ResponseWriter, r *http.Request) {
	if !m.PasswordEnabled() {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if !sameOrigin(r) {
		http.Error(w, "cross-origin login rejected", http.StatusForbidden)
		return
	}
	type body struct {
		Password string `json:"password"`
	}
	var b body
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if err := m.Verify(b.Password); err != nil {
		// Constant time check already; uniform error.
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	tok, err := m.Issue(SessionTTL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    tok,
		Path:     "/",
		Expires:  time.Now().Add(SessionTTL),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// sameOrigin returns true if the request looks like it came from the same
// origin as the server, or if there's no Origin/Referer to check (e.g.
// curl/test/native API client). For browser flows, both Origin and Referer
// are normally set.
func sameOrigin(r *http.Request) bool {
	for _, hdr := range []string{"Origin", "Referer"} {
		v := r.Header.Get(hdr)
		if v == "" {
			continue
		}
		u, err := urlParse(v)
		if err != nil {
			return false
		}
		if u.Host != r.Host {
			return false
		}
		// First non-empty header wins; if it matches, accept.
		return true
	}
	// No Origin and no Referer: not a browser, treat as trusted (the API
	// token middleware handles non-browser cases anyway).
	return true
}

// LogoutHandler clears the cookie.
func (m *Manager) LogoutHandler(w http.ResponseWriter, _ *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
		HttpOnly: true,
	})
	w.WriteHeader(http.StatusNoContent)
}

// StatusHandler returns whether auth is enabled and whether the requester is
// currently logged in. Always public.
//
// `enabled` is true if EITHER cookie or token auth is configured. The
// frontend uses this to decide whether to render the login screen — it
// only needs the cookie path, since browsers can't easily attach Bearer
// tokens to subsequent fetches without cooperation.
func (m *Manager) StatusHandler(w http.ResponseWriter, r *http.Request) {
	resp := map[string]bool{
		"enabled":         m.Enabled(),
		"passwordEnabled": m.PasswordEnabled(),
		"loggedIn":        false,
	}
	if !m.Enabled() {
		resp["loggedIn"] = true
	} else if c, err := r.Cookie(CookieName); err == nil && m.Validate(c.Value) == nil {
		resp["loggedIn"] = true
	} else if m.hasValidToken(r.Header.Get("Authorization")) {
		resp["loggedIn"] = true
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
