// oidc_handler.go wires OIDCProvider into the existing Manager / cookie
// session machinery.
//
// Two routes:
//
//   GET  /api/auth/oidc/start    → redirect to IdP
//   GET  /api/auth/oidc/callback → exchange code, verify ID token, set cookie
//
// We store (state, code_verifier) in a short-lived signed cookie keyed by
// the OIDC manager's HMAC key. The IdP echoes state back; we verify it on
// callback to defeat CSRF.

package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

// OIDCManager wraps a Manager + OIDCProvider into a small handler bundle.
type OIDCManager struct {
	M *Manager
	P *OIDCProvider
}

// flowState is what we stash in the round-trip cookie.
type flowState struct {
	State        string `json:"s"`
	CodeVerifier string `json:"v"`
	Issued       int64  `json:"t"`
}

// flowCookieName is the round-trip cookie set during the OIDC flow.
const flowCookieName = "sunny_oidc_flow"

// flowCookieTTL caps how long the user has to complete the flow with the IdP.
// 10 minutes is generous; most IdPs complete in under a minute.
const flowCookieTTL = 10 * time.Minute

// StartHandler kicks off the OIDC flow. GET only.
func (o *OIDCManager) StartHandler(w http.ResponseWriter, r *http.Request) {
	if o.P == nil {
		http.Error(w, "oidc not configured", http.StatusServiceUnavailable)
		return
	}
	state := NewState()
	verifier := NewCodeVerifier()
	fs := flowState{State: state, CodeVerifier: verifier, Issued: time.Now().Unix()}
	cookie, err := o.signFlow(fs)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     flowCookieName,
		Value:    cookie,
		Path:     "/",
		Expires:  time.Now().Add(flowCookieTTL),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
	})
	http.Redirect(w, r, o.P.AuthCodeURL(state, verifier), http.StatusFound)
}

// CallbackHandler completes the OIDC flow. GET, expects code + state in the
// query string from the IdP.
func (o *OIDCManager) CallbackHandler(w http.ResponseWriter, r *http.Request) {
	if o.P == nil {
		http.Error(w, "oidc not configured", http.StatusServiceUnavailable)
		return
	}
	q := r.URL.Query()
	code := q.Get("code")
	state := q.Get("state")
	if code == "" || state == "" {
		http.Error(w, "missing code or state", http.StatusBadRequest)
		return
	}
	c, err := r.Cookie(flowCookieName)
	if err != nil {
		http.Error(w, "missing flow cookie", http.StatusBadRequest)
		return
	}
	fs, err := o.verifyFlow(c.Value)
	if err != nil {
		http.Error(w, "invalid flow cookie", http.StatusBadRequest)
		return
	}
	if fs.State != state {
		http.Error(w, "state mismatch", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	tok, err := o.P.Exchange(ctx, code, fs.CodeVerifier)
	if err != nil {
		http.Error(w, "token exchange failed: "+err.Error(), http.StatusUnauthorized)
		return
	}
	if _, err := o.P.VerifyIDToken(ctx, tok.IDToken); err != nil {
		http.Error(w, "id_token verify failed: "+err.Error(), http.StatusUnauthorized)
		return
	}
	// Mint a Sunny session cookie.
	session, err := o.M.Issue(SessionTTL)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    session,
		Path:     "/",
		Expires:  time.Now().Add(SessionTTL),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil,
	})
	// Clear the flow cookie.
	http.SetCookie(w, &http.Cookie{
		Name: flowCookieName, Value: "", Path: "/",
		Expires: time.Unix(0, 0), MaxAge: -1, HttpOnly: true,
	})
	// Send the browser back to the SPA root.
	http.Redirect(w, r, "/", http.StatusFound)
}

// signFlow encodes + HMACs the flow state using the manager's HMAC key.
func (o *OIDCManager) signFlow(fs flowState) (string, error) {
	body, err := json.Marshal(fs)
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(body)
	mac := hmacHex(o.M.hmacKey, encoded)
	return encoded + "." + mac, nil
}

// verifyFlow inverts signFlow. Also enforces the cookie TTL.
func (o *OIDCManager) verifyFlow(token string) (flowState, error) {
	dot := -1
	for i := len(token) - 1; i >= 0; i-- {
		if token[i] == '.' {
			dot = i
			break
		}
	}
	if dot <= 0 {
		return flowState{}, errors.New("malformed flow token")
	}
	encoded, mac := token[:dot], token[dot+1:]
	if hmacHex(o.M.hmacKey, encoded) != mac {
		return flowState{}, errors.New("bad flow signature")
	}
	body, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return flowState{}, err
	}
	var fs flowState
	if err := json.Unmarshal(body, &fs); err != nil {
		return flowState{}, err
	}
	if time.Now().Unix()-fs.Issued > int64(flowCookieTTL.Seconds()) {
		return flowState{}, errors.New("flow cookie expired")
	}
	return fs, nil
}

// hmacHex computes HMAC-SHA256(key, msg) and returns lowercase hex. Same
// construction as Manager.Issue uses for session cookies.
func hmacHex(key []byte, msg string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(msg))
	return hex.EncodeToString(m.Sum(nil))
}
