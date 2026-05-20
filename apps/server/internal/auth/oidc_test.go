package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNewState_RandomEachTime(t *testing.T) {
	t.Parallel()
	a, b := NewState(), NewState()
	if a == b {
		t.Fatalf("NewState repeated: %s", a)
	}
	if len(a) != 32 { // 16 bytes hex
		t.Fatalf("len = %d, want 32", len(a))
	}
}

func TestNewCodeVerifier_LengthAndChallenge(t *testing.T) {
	t.Parallel()
	v := NewCodeVerifier()
	if len(v) < 43 { // base64url of 32 bytes
		t.Fatalf("verifier len = %d", len(v))
	}
	c := pkceChallenge(v)
	if c == v {
		t.Fatalf("challenge should differ from verifier")
	}
	// Idempotent.
	if c2 := pkceChallenge(v); c2 != c {
		t.Fatalf("pkceChallenge not deterministic")
	}
}

// fakeIdP is an httptest-backed minimal OIDC server. It signs ID tokens with
// a self-generated RSA key and serves discovery + JWKS.
type fakeIdP struct {
	t        *testing.T
	srv      *httptest.Server
	priv     *rsa.PrivateKey
	kid      string
	clientID string
	issuer   string

	// programmable response from the token endpoint
	idTokenOverride string
}

func newFakeIdP(t *testing.T, clientID string) *fakeIdP {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeIdP{t: t, priv: priv, kid: "kid-1", clientID: clientID}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{
			"issuer":                 f.issuer,
			"authorization_endpoint": f.issuer + "/authorize",
			"token_endpoint":         f.issuer + "/token",
			"jwks_uri":               f.issuer + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		n := f.priv.PublicKey.N.Bytes()
		var eBuf [8]byte
		binary.BigEndian.PutUint64(eBuf[:], uint64(f.priv.PublicKey.E))
		// trim leading zeros to match wire encoding
		i := 0
		for i < len(eBuf) && eBuf[i] == 0 {
			i++
		}
		e := eBuf[i:]
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]string{{
				"kty": "RSA", "kid": f.kid, "alg": "RS256", "use": "sig",
				"n": base64.RawURLEncoding.EncodeToString(n),
				"e": base64.RawURLEncoding.EncodeToString(e),
			}},
		})
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		idToken := f.idTokenOverride
		if idToken == "" {
			idToken = f.signIDToken(IDTokenClaims{
				Issuer:   f.issuer,
				Subject:  "user-42",
				Audience: f.clientID,
				Expiry:   time.Now().Add(5 * time.Minute).Unix(),
				IssuedAt: time.Now().Unix(),
				Email:    "u@example.com",
			})
		}
		_ = json.NewEncoder(w).Encode(TokenResponse{
			AccessToken: "fake-access",
			IDToken:     idToken,
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	f.srv = srv
	f.issuer = srv.URL
	return f
}

func (f *fakeIdP) signIDToken(c IDTokenClaims) string {
	header := map[string]string{"alg": "RS256", "kid": f.kid, "typ": "JWT"}
	hb, _ := json.Marshal(header)
	pb, _ := json.Marshal(c)
	signingInput := base64.RawURLEncoding.EncodeToString(hb) + "." + base64.RawURLEncoding.EncodeToString(pb)
	hashed := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, f.priv, crypto.SHA256, hashed[:])
	if err != nil {
		f.t.Fatal(err)
	}
	return signingInput + "." + base64.RawURLEncoding.EncodeToString(sig)
}

func TestOIDC_DiscoveryAndVerify(t *testing.T) {
	t.Parallel()
	idp := newFakeIdP(t, "test-client")
	p, err := NewOIDCProvider(context.Background(), OIDCConfig{
		Issuer:      idp.issuer,
		ClientID:    "test-client",
		RedirectURL: "https://app.example.com/api/auth/oidc/callback",
	})
	if err != nil {
		t.Fatalf("NewOIDCProvider: %v", err)
	}
	token := idp.signIDToken(IDTokenClaims{
		Issuer:   idp.issuer,
		Subject:  "alice",
		Audience: "test-client",
		Expiry:   time.Now().Add(time.Minute).Unix(),
		IssuedAt: time.Now().Unix(),
		Email:    "alice@example.com",
	})
	claims, err := p.VerifyIDToken(context.Background(), token)
	if err != nil {
		t.Fatalf("VerifyIDToken: %v", err)
	}
	if claims.Subject != "alice" {
		t.Fatalf("sub = %q", claims.Subject)
	}
	if claims.Email != "alice@example.com" {
		t.Fatalf("email = %q", claims.Email)
	}
}

func TestOIDC_RejectsBadAudience(t *testing.T) {
	t.Parallel()
	idp := newFakeIdP(t, "test-client")
	p, _ := NewOIDCProvider(context.Background(), OIDCConfig{
		Issuer: idp.issuer, ClientID: "test-client", RedirectURL: "https://x/cb",
	})
	tok := idp.signIDToken(IDTokenClaims{
		Issuer:   idp.issuer,
		Subject:  "alice",
		Audience: "other-client", // wrong
		Expiry:   time.Now().Add(time.Minute).Unix(),
	})
	if _, err := p.VerifyIDToken(context.Background(), tok); err == nil {
		t.Fatal("expected audience rejection")
	}
}

func TestOIDC_RejectsExpired(t *testing.T) {
	t.Parallel()
	idp := newFakeIdP(t, "test-client")
	p, _ := NewOIDCProvider(context.Background(), OIDCConfig{
		Issuer: idp.issuer, ClientID: "test-client", RedirectURL: "https://x/cb",
	})
	tok := idp.signIDToken(IDTokenClaims{
		Issuer:   idp.issuer,
		Subject:  "alice",
		Audience: "test-client",
		Expiry:   time.Now().Add(-time.Minute).Unix(),
	})
	if _, err := p.VerifyIDToken(context.Background(), tok); err == nil {
		t.Fatal("expected expiry rejection")
	}
}

func TestOIDC_RejectsBadSignature(t *testing.T) {
	t.Parallel()
	idp := newFakeIdP(t, "test-client")
	p, _ := NewOIDCProvider(context.Background(), OIDCConfig{
		Issuer: idp.issuer, ClientID: "test-client", RedirectURL: "https://x/cb",
	})
	tok := idp.signIDToken(IDTokenClaims{
		Issuer: idp.issuer, Subject: "alice", Audience: "test-client",
		Expiry: time.Now().Add(time.Minute).Unix(),
	})
	// flip the last byte of the signature
	parts := strings.Split(tok, ".")
	sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
	sig[len(sig)-1] ^= 1
	parts[2] = base64.RawURLEncoding.EncodeToString(sig)
	tok = strings.Join(parts, ".")
	if _, err := p.VerifyIDToken(context.Background(), tok); err == nil {
		t.Fatal("expected signature rejection")
	}
}

func TestOIDC_StartHandlerSetsFlowCookie(t *testing.T) {
	t.Parallel()
	idp := newFakeIdP(t, "test-client")
	p, _ := NewOIDCProvider(context.Background(), OIDCConfig{
		Issuer: idp.issuer, ClientID: "test-client", RedirectURL: "https://x/cb",
	})
	mgr, _ := NewManager("", "", "")
	om := &OIDCManager{M: mgr, P: p}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/start", nil)
	om.StartHandler(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d", rec.Code)
	}
	loc := rec.Header().Get("Location")
	u, err := url.Parse(loc)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"code_challenge", "code_challenge_method=S256", "state="} {
		if !strings.Contains(u.RawQuery, want) {
			t.Errorf("authorize URL missing %q", want)
		}
	}
	var foundCookie bool
	for _, c := range rec.Result().Cookies() {
		if c.Name == flowCookieName && c.Value != "" {
			foundCookie = true
		}
	}
	if !foundCookie {
		t.Fatalf("flow cookie not set")
	}
}

func TestOIDC_CallbackFullFlow(t *testing.T) {
	t.Parallel()
	idp := newFakeIdP(t, "test-client")
	p, _ := NewOIDCProvider(context.Background(), OIDCConfig{
		Issuer: idp.issuer, ClientID: "test-client", RedirectURL: "https://x/cb",
	})
	mgr, _ := NewManager("", "supersecret-key-for-flow", "")
	om := &OIDCManager{M: mgr, P: p}

	// 1) start: capture state + cookie
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/start", nil)
	om.StartHandler(rec, req)
	loc, _ := url.Parse(rec.Header().Get("Location"))
	state := loc.Query().Get("state")
	var flow *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == flowCookieName {
			flow = c
		}
	}
	if flow == nil {
		t.Fatal("missing flow cookie")
	}

	// 2) callback
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet,
		fmt.Sprintf("/api/auth/oidc/callback?code=ok&state=%s", state), nil)
	req2.AddCookie(flow)
	om.CallbackHandler(rec2, req2)
	if rec2.Code != http.StatusFound {
		t.Fatalf("callback status = %d, body = %s", rec2.Code, rec2.Body.String())
	}
	var session *http.Cookie
	for _, c := range rec2.Result().Cookies() {
		if c.Name == CookieName {
			session = c
		}
	}
	if session == nil {
		t.Fatal("session cookie not issued")
	}
	if err := mgr.Validate(session.Value); err != nil {
		t.Fatalf("session validate: %v", err)
	}
}

func TestOIDC_CallbackRejectsStateMismatch(t *testing.T) {
	t.Parallel()
	idp := newFakeIdP(t, "test-client")
	p, _ := NewOIDCProvider(context.Background(), OIDCConfig{
		Issuer: idp.issuer, ClientID: "test-client", RedirectURL: "https://x/cb",
	})
	mgr, _ := NewManager("", "supersecret-key", "")
	om := &OIDCManager{M: mgr, P: p}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/start", nil)
	om.StartHandler(rec, req)
	var flow *http.Cookie
	for _, c := range rec.Result().Cookies() {
		if c.Name == flowCookieName {
			flow = c
		}
	}
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/callback?code=ok&state=different", nil)
	req2.AddCookie(flow)
	om.CallbackHandler(rec2, req2)
	if rec2.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec2.Code)
	}
}
