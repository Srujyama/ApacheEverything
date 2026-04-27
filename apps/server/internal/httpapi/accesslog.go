package httpapi

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

// accessLogMiddleware logs every API request as a structured slog line.
// Skips /api/health and /api/version to keep the log volume sane —
// liveness probes and dashboard auto-pollers shouldn't drown the log.
//
// We log at INFO for 2xx/3xx, WARN for 4xx, ERROR for 5xx. Slow requests
// (>500ms) get a "slow=true" attribute regardless of status.
func accessLogMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if shouldSkipLog(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()
			next.ServeHTTP(ww, r)
			elapsed := time.Since(start)

			status := ww.Status()
			if status == 0 {
				status = 200
			}
			level := slog.LevelInfo
			switch {
			case status >= 500:
				level = slog.LevelError
			case status >= 400:
				level = slog.LevelWarn
			}

			attrs := []any{
				"method", r.Method,
				"path", r.URL.Path,
				"status", status,
				"duration_ms", elapsed.Milliseconds(),
				"bytes", ww.BytesWritten(),
				"remote", r.RemoteAddr,
				"request_id", middleware.GetReqID(r.Context()),
			}
			if elapsed > 500*time.Millisecond {
				attrs = append(attrs, "slow", true)
			}
			logger.Log(r.Context(), level, "http", attrs...)
		})
	}
}

// shouldSkipLog returns true for paths whose volume isn't worth logging.
// We keep /api/stream (long-lived, one log per connect) and /api/auth
// (security-relevant) on purpose.
func shouldSkipLog(path string) bool {
	switch path {
	case "/api/health", "/api/version":
		return true
	}
	// Static assets — embedded SPA. Most are fingerprinted and cached.
	if strings.HasPrefix(path, "/assets/") {
		return true
	}
	return false
}
