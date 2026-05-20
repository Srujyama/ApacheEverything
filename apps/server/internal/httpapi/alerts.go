package httpapi

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/sunny/sunny/apps/server/internal/alerts"
	"github.com/sunny/sunny/apps/server/internal/storage"
)

type alertsAPI struct {
	store storage.Storage
	dlq   alerts.DeadLetterStore // optional; nil → /alerts/deadletters returns 503
}

// GET /api/alerts?limit=
func (a *alertsAPI) listAlerts(w http.ResponseWriter, r *http.Request) {
	limit := parseIntParam(r.URL.Query().Get("limit"), 100, 1, 1000)
	out, err := a.store.ListAlerts(r.Context(), limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if out == nil {
		out = []storage.Alert{}
	}
	writeJSON(w, http.StatusOK, out)
}

// POST /api/alerts/{id}/ack
func (a *alertsAPI) ackAlert(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := a.store.AckAlert(r.Context(), id, time.Now().UTC()); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "acked", "id": id})
}

// GET /api/alerts/rules
func (a *alertsAPI) listRules(w http.ResponseWriter, r *http.Request) {
	out, err := a.store.ListRules(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if out == nil {
		out = []storage.AlertRule{}
	}
	writeJSON(w, http.StatusOK, out)
}

// POST /api/alerts/rules — body is an AlertRule. ID auto-generated if empty.
func (a *alertsAPI) saveRule(w http.ResponseWriter, r *http.Request) {
	var rule storage.AlertRule
	if err := json.NewDecoder(r.Body).Decode(&rule); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(rule.Name) == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if rule.ID == "" {
		var b [8]byte
		_, _ = rand.Read(b[:])
		rule.ID = "rule-" + hex.EncodeToString(b[:])
	}
	if err := a.store.SaveRule(r.Context(), rule); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, rule)
}

// DELETE /api/alerts/rules/{id}
func (a *alertsAPI) deleteRule(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := a.store.DeleteRule(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusNoContent, map[string]string{})
}

// GET /api/alerts/deadletters?limit=
//
// Returns alerts that exhausted retry attempts on their notifiers. Useful
// for operators chasing down a broken Slack webhook or a webhook receiver
// that's gone offline. Returns 503 if no DLQ is configured.
func (a *alertsAPI) listDeadLetters(w http.ResponseWriter, r *http.Request) {
	if a.dlq == nil {
		http.Error(w, "dead-letter store not configured", http.StatusServiceUnavailable)
		return
	}
	limit := parseIntParam(r.URL.Query().Get("limit"), 100, 1, 1000)
	out, err := a.dlq.ListDeadLetters(r.Context(), limit)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if out == nil {
		out = []alerts.DeadLetter{}
	}
	writeJSON(w, http.StatusOK, out)
}
