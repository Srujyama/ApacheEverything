package httpapi

import (
	"net/http"
	"strings"
)

// corsConfig holds the parsed allowed-origin set.
type corsConfig struct {
	allowAll bool
	origins  map[string]struct{}
}

// parseCORS reads the SUNNY_CORS_ORIGINS-style value:
//
//   - empty or unset → CORS disabled (no headers added).
//   - "*"            → allow any origin (echoes Origin back; doesn't use "*" so credentials work).
//   - comma list     → exact-match against the request's Origin.
func parseCORS(s string) *corsConfig {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	if s == "*" {
		return &corsConfig{allowAll: true, origins: map[string]struct{}{}}
	}
	c := &corsConfig{origins: map[string]struct{}{}}
	for _, o := range strings.Split(s, ",") {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		c.origins[o] = struct{}{}
	}
	return c
}

func (c *corsConfig) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" && c.allowed(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin")
			if r.Method == http.MethodOptions {
				// Preflight.
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
				reqHeaders := r.Header.Get("Access-Control-Request-Headers")
				if reqHeaders == "" {
					reqHeaders = "Content-Type, Authorization, X-Sunny-Token"
				}
				w.Header().Set("Access-Control-Allow-Headers", reqHeaders)
				w.Header().Set("Access-Control-Max-Age", "600")
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (c *corsConfig) allowed(origin string) bool {
	if c.allowAll {
		return true
	}
	_, ok := c.origins[origin]
	return ok
}
