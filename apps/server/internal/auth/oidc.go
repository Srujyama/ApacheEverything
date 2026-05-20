// oidc.go implements a minimal OpenID Connect Authorization Code + PKCE
// client suitable for self-hosted Sunny deployments.
//
// Scope: just enough to let users log in via an external IdP (Okta, Auth0,
// Keycloak, Authentik, Google, GitHub via dex, …). We do NOT implement:
//   - dynamic client registration
//   - back-channel logout
//   - userinfo endpoint (we read claims directly from the ID token)
//   - encrypted ID tokens
//
// Configured by env at startup (Sunny is single-tenant; multi-tenant lives
// in Phase 11):
//
//   SUNNY_OIDC_ISSUER     — required, e.g. https://acme.okta.com
//   SUNNY_OIDC_CLIENT_ID  — required
//   SUNNY_OIDC_CLIENT_SECRET — required for confidential clients (web apps)
//   SUNNY_OIDC_REDIRECT_URL — required, e.g. https://sunny.acme.com/api/auth/oidc/callback
//   SUNNY_OIDC_SCOPES     — optional, default "openid profile email"
//
// On successful login Sunny issues its own session cookie (same as the
// password flow) — we don't re-validate the ID token on every request,
// because doing so against the JWKS endpoint hits the IdP for every call.
// The session cookie has the same TTL semantics as the password flow.

package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// OIDCConfig is everything we need to drive an OIDC flow.
type OIDCConfig struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string
	Scopes       []string // default ["openid", "profile", "email"]
}

// Validate normalizes and checks the config.
func (c *OIDCConfig) Validate() error {
	if c.Issuer == "" {
		return errors.New("oidc: missing SUNNY_OIDC_ISSUER")
	}
	if c.ClientID == "" {
		return errors.New("oidc: missing SUNNY_OIDC_CLIENT_ID")
	}
	if c.RedirectURL == "" {
		return errors.New("oidc: missing SUNNY_OIDC_REDIRECT_URL")
	}
	if len(c.Scopes) == 0 {
		c.Scopes = []string{"openid", "profile", "email"}
	}
	return nil
}

// OIDCProvider holds the discovered metadata + a JWKS cache.
type OIDCProvider struct {
	Config    OIDCConfig
	Discovery providerDiscovery

	httpClient *http.Client

	jwksMu       sync.RWMutex
	jwksCache    map[string]*rsa.PublicKey // kid → key
	jwksFetched  time.Time
	jwksMaxAge   time.Duration
}

type providerDiscovery struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JWKSURI               string `json:"jwks_uri"`
}

// NewOIDCProvider performs discovery and returns a ready-to-use provider.
// The HTTP timeout is short — discovery should be near-instant; if your IdP
// is slow, that's a different problem.
func NewOIDCProvider(ctx context.Context, cfg OIDCConfig) (*OIDCProvider, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	hc := &http.Client{Timeout: 10 * time.Second}
	disco, err := discover(ctx, hc, cfg.Issuer)
	if err != nil {
		return nil, err
	}
	if disco.Issuer != cfg.Issuer && disco.Issuer+"/" != cfg.Issuer {
		// Some IdPs canonicalize differently — accept exact or with slash.
		return nil, fmt.Errorf("oidc: issuer mismatch: configured %q, discovery says %q", cfg.Issuer, disco.Issuer)
	}
	return &OIDCProvider{
		Config:     cfg,
		Discovery:  disco,
		httpClient: hc,
		jwksCache:  map[string]*rsa.PublicKey{},
		jwksMaxAge: 10 * time.Minute,
	}, nil
}

// discover fetches /.well-known/openid-configuration.
func discover(ctx context.Context, hc *http.Client, issuer string) (providerDiscovery, error) {
	u := strings.TrimRight(issuer, "/") + "/.well-known/openid-configuration"
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	res, err := hc.Do(req)
	if err != nil {
		return providerDiscovery{}, fmt.Errorf("oidc: discovery: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return providerDiscovery{}, fmt.Errorf("oidc: discovery: %s", res.Status)
	}
	var d providerDiscovery
	if err := json.NewDecoder(res.Body).Decode(&d); err != nil {
		return providerDiscovery{}, fmt.Errorf("oidc: discovery decode: %w", err)
	}
	if d.AuthorizationEndpoint == "" || d.TokenEndpoint == "" || d.JWKSURI == "" {
		return providerDiscovery{}, errors.New("oidc: discovery missing required endpoints")
	}
	return d, nil
}

// AuthCodeURL builds the URL to redirect the user to. The returned state
// and code_verifier should be stored in a short-lived cookie so the
// callback can validate them.
func (p *OIDCProvider) AuthCodeURL(state, codeVerifier string) string {
	challenge := pkceChallenge(codeVerifier)
	q := url.Values{}
	q.Set("client_id", p.Config.ClientID)
	q.Set("response_type", "code")
	q.Set("redirect_uri", p.Config.RedirectURL)
	q.Set("scope", strings.Join(p.Config.Scopes, " "))
	q.Set("state", state)
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	return p.Discovery.AuthorizationEndpoint + "?" + q.Encode()
}

// TokenResponse is the subset of the token endpoint we care about.
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	IDToken     string `json:"id_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

// Exchange swaps an authorization code for tokens.
func (p *OIDCProvider) Exchange(ctx context.Context, code, codeVerifier string) (TokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	form.Set("redirect_uri", p.Config.RedirectURL)
	form.Set("client_id", p.Config.ClientID)
	form.Set("code_verifier", codeVerifier)
	if p.Config.ClientSecret != "" {
		form.Set("client_secret", p.Config.ClientSecret)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, p.Discovery.TokenEndpoint, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := p.httpClient.Do(req)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("oidc: token exchange: %w", err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 {
		return TokenResponse{}, fmt.Errorf("oidc: token exchange: %s: %s", res.Status, body)
	}
	var tok TokenResponse
	if err := json.Unmarshal(body, &tok); err != nil {
		return TokenResponse{}, fmt.Errorf("oidc: token decode: %w", err)
	}
	if tok.IDToken == "" {
		return TokenResponse{}, errors.New("oidc: token response missing id_token")
	}
	return tok, nil
}

// IDTokenClaims is the subset we extract from the ID token. Extend as needed.
type IDTokenClaims struct {
	Issuer    string `json:"iss"`
	Subject   string `json:"sub"`
	Audience  any    `json:"aud"` // string or []string per spec
	Expiry    int64  `json:"exp"`
	IssuedAt  int64  `json:"iat"`
	Email     string `json:"email"`
	Name      string `json:"name"`
	Picture   string `json:"picture"`
	Nonce     string `json:"nonce,omitempty"`
}

// AudienceMatches returns true if want appears in the token's aud claim.
func (c IDTokenClaims) AudienceMatches(want string) bool {
	switch v := c.Audience.(type) {
	case string:
		return v == want
	case []any:
		for _, a := range v {
			if s, ok := a.(string); ok && s == want {
				return true
			}
		}
	case []string:
		for _, s := range v {
			if s == want {
				return true
			}
		}
	}
	return false
}

// VerifyIDToken validates the ID token's signature, issuer, audience, and
// expiry. Returns the claims on success.
func (p *OIDCProvider) VerifyIDToken(ctx context.Context, idToken string) (IDTokenClaims, error) {
	parts := strings.Split(idToken, ".")
	if len(parts) != 3 {
		return IDTokenClaims{}, errors.New("oidc: malformed JWT")
	}
	headerB, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return IDTokenClaims{}, fmt.Errorf("oidc: decode header: %w", err)
	}
	payloadB, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return IDTokenClaims{}, fmt.Errorf("oidc: decode payload: %w", err)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return IDTokenClaims{}, fmt.Errorf("oidc: decode signature: %w", err)
	}

	var hdr struct {
		Alg string `json:"alg"`
		Kid string `json:"kid"`
	}
	if err := json.Unmarshal(headerB, &hdr); err != nil {
		return IDTokenClaims{}, fmt.Errorf("oidc: decode header json: %w", err)
	}
	if hdr.Alg != "RS256" {
		return IDTokenClaims{}, fmt.Errorf("oidc: unsupported alg %q (only RS256)", hdr.Alg)
	}

	key, err := p.publicKey(ctx, hdr.Kid)
	if err != nil {
		return IDTokenClaims{}, err
	}
	signingInput := []byte(parts[0] + "." + parts[1])
	hashed := sha256.Sum256(signingInput)
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, hashed[:], sig); err != nil {
		return IDTokenClaims{}, fmt.Errorf("oidc: signature verify: %w", err)
	}

	var claims IDTokenClaims
	if err := json.Unmarshal(payloadB, &claims); err != nil {
		return IDTokenClaims{}, fmt.Errorf("oidc: decode claims: %w", err)
	}
	if claims.Issuer != p.Config.Issuer && claims.Issuer+"/" != p.Config.Issuer {
		return claims, fmt.Errorf("oidc: issuer mismatch: %q vs %q", claims.Issuer, p.Config.Issuer)
	}
	if !claims.AudienceMatches(p.Config.ClientID) {
		return claims, fmt.Errorf("oidc: audience mismatch; expected %q", p.Config.ClientID)
	}
	if claims.Expiry == 0 || time.Now().Unix() > claims.Expiry {
		return claims, errors.New("oidc: token expired")
	}
	return claims, nil
}

// publicKey returns the RSA public key for kid, refreshing the JWKS cache
// if necessary.
func (p *OIDCProvider) publicKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	p.jwksMu.RLock()
	k, ok := p.jwksCache[kid]
	fresh := time.Since(p.jwksFetched) < p.jwksMaxAge
	p.jwksMu.RUnlock()
	if ok && fresh {
		return k, nil
	}
	if err := p.refreshJWKS(ctx); err != nil {
		return nil, err
	}
	p.jwksMu.RLock()
	defer p.jwksMu.RUnlock()
	k, ok = p.jwksCache[kid]
	if !ok {
		return nil, fmt.Errorf("oidc: kid %q not in JWKS", kid)
	}
	return k, nil
}

type jwksKey struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwksResponse struct {
	Keys []jwksKey `json:"keys"`
}

func (p *OIDCProvider) refreshJWKS(ctx context.Context) error {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, p.Discovery.JWKSURI, nil)
	res, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("oidc: fetch jwks: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		return fmt.Errorf("oidc: jwks: %s", res.Status)
	}
	var jr jwksResponse
	if err := json.NewDecoder(res.Body).Decode(&jr); err != nil {
		return fmt.Errorf("oidc: decode jwks: %w", err)
	}
	p.jwksMu.Lock()
	defer p.jwksMu.Unlock()
	p.jwksCache = map[string]*rsa.PublicKey{}
	for _, k := range jr.Keys {
		if k.Kty != "RSA" {
			continue
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		// 'e' is typically 65537 (0x010001). Pad to 8 bytes for the int parser.
		if len(eBytes) > 8 {
			continue
		}
		var eBuf [8]byte
		copy(eBuf[8-len(eBytes):], eBytes)
		e := binary.BigEndian.Uint64(eBuf[:])
		p.jwksCache[k.Kid] = &rsa.PublicKey{
			N: new(big.Int).SetBytes(nBytes),
			E: int(e),
		}
	}
	p.jwksFetched = time.Now()
	return nil
}

// ---------------------------------------------------------------------------
// PKCE + state helpers
// ---------------------------------------------------------------------------

// NewState returns a random opaque string suitable for CSRF protection.
func NewState() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// NewCodeVerifier returns a high-entropy PKCE code verifier (RFC 7636).
func NewCodeVerifier() string {
	var b [32]byte
	_, _ = rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}

// pkceChallenge returns S256(verifier).
func pkceChallenge(verifier string) string {
	h := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(h[:])
}

