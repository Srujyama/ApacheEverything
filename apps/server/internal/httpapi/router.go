package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/sunny/sunny/apps/server/internal/auth"
	"github.com/sunny/sunny/apps/server/internal/bus"
	"github.com/sunny/sunny/apps/server/internal/connectors"
	"github.com/sunny/sunny/apps/server/internal/storage"
	"github.com/sunny/sunny/apps/server/internal/web"
)

const Version = "0.1.0"

// Deps is what NewRouter needs from main. Keeping it explicit makes the
// router unit-testable without touching globals.
type Deps struct {
	Logger      *slog.Logger
	Runtime     *connectors.Runtime
	Bus         *bus.Bus
	Storage     storage.Storage
	Auth        *auth.Manager
	DataDir     string // for /api/backup; if empty, the endpoint returns 503.
	QueryRPM    int    // requests per minute per IP for /api/query and /api/export. 0 → default 10.
	CORSOrigins string // comma-separated list of allowed origins, or "*". Empty disables CORS.
}

func NewRouter(d Deps) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	if d.Logger != nil {
		r.Use(accessLogMiddleware(d.Logger))
	}
	if c := parseCORS(d.CORSOrigins); c != nil {
		r.Use(c.middleware)
	}

	records := &recordsAPI{store: d.Storage}
	ts := &timeseriesAPI{store: d.Storage}
	alerts := &alertsAPI{store: d.Storage}
	exp := &exportAPI{store: d.Storage}
	qry := &queryAPI{store: d.Storage}
	met := &metricsAPI{runtime: d.Runtime, store: d.Storage}
	bak := &backupAPI{dataDir: d.DataDir}

	r.Route("/api", func(r chi.Router) {
		// Always-public routes.
		r.Get("/health", healthHandler)
		r.Get("/version", versionHandler)
		r.Get("/openapi.yaml", openapiSpecHandler)
		r.Get("/docs", apiDocsHandler)
		if d.Auth != nil {
			r.Get("/auth/status", d.Auth.StatusHandler)
			r.Post("/auth/login", d.Auth.LoginHandler)
			r.Post("/auth/logout", d.Auth.LogoutHandler)
		}

		// Authenticated routes (no-op middleware if auth disabled).
		r.Group(func(r chi.Router) {
			if d.Auth != nil {
				r.Use(d.Auth.Middleware)
			}
			mountConnectorRoutes(r, d.Runtime, d.Bus)

			r.Get("/records", records.list)
			r.Get("/records/recent", records.list) // phase-1 alias
			r.Get("/records/counts", ts.counts)
			r.Get("/timeseries", ts.timeseries)

			// /export and /query are heavy — guard with a per-IP rate limit.
			heavy := newRateLimiter(d.QueryRPM)
			r.Group(func(r chi.Router) {
				r.Use(heavy.middleware)
				r.Get("/export", exp.handle)
				r.Post("/query", qry.handle)
			})

			r.Get("/connectors/{id}/metrics", met.handle)

			// /backup is auth-protected, rate-limited (it reads the whole DB).
			if d.DataDir != "" {
				r.Group(func(r chi.Router) {
					r.Use(newRateLimiter(2).middleware) // tight: 2 backups / min
					r.Get("/backup", bak.handle)
					r.Post("/backup", bak.handle)
				})
			}

			r.Get("/alerts", alerts.listAlerts)
			r.Post("/alerts/{id}/ack", alerts.ackAlert)
			r.Get("/alerts/rules", alerts.listRules)
			r.Post("/alerts/rules", alerts.saveRule)
			r.Delete("/alerts/rules/{id}", alerts.deleteRule)
		})

		// Push-connector ingest path. Intentionally NOT behind the session-
		// cookie auth: webhook callers (other systems, vendor automation)
		// can't carry a browser cookie. Each push connector enforces its
		// own token via X-Sunny-Token instead. See connectors/webhook.
		r.HandleFunc("/ingest/{id}", pushHandler(d.Runtime))
		r.HandleFunc("/ingest/{id}/*", pushHandler(d.Runtime))
	})

	// Kubernetes-style liveness / readiness probes. Public, outside /api so
	// probes don't trip auth, CORS, or rate limits.
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok"))
	})
	r.Get("/readyz", func(w http.ResponseWriter, r *http.Request) {
		// Ready if storage answers a trivial query and at least one
		// connector instance is in a non-failed state. New deployments
		// with zero instances configured still count as ready (the
		// runtime is up; users just haven't added connectors yet).
		if d.Storage != nil {
			if _, err := d.Storage.Recent(r.Context(), 1); err != nil {
				http.Error(w, "storage not ready: "+err.Error(), http.StatusServiceUnavailable)
				return
			}
		}
		if d.Runtime != nil {
			for _, st := range d.Runtime.Statuses() {
				if st.State == connectors.StateFailed {
					http.Error(w, "instance "+st.InstanceID+" failed", http.StatusServiceUnavailable)
					return
				}
			}
		}
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok"))
	})

	web.Mount(r)

	return r
}

func healthHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

func versionHandler(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"version": Version,
		"phase":   "v0.1",
	})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
