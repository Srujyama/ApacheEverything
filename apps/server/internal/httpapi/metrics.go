package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/sunny/sunny/apps/server/internal/connectors"
	"github.com/sunny/sunny/apps/server/internal/storage"
)

// InstanceMetrics is the wire shape of /api/connectors/{id}/metrics.
type InstanceMetrics struct {
	InstanceID    string     `json:"instanceId"`
	State         string     `json:"state"`
	Restarts      int        `json:"restarts"`
	StartedAt     time.Time  `json:"startedAt"`
	LastError     string     `json:"lastError,omitempty"`
	LastErrorAt   *time.Time `json:"lastErrorAt,omitempty"`
	TotalRecords  int64      `json:"totalRecords"`
	LastRecordAt  *time.Time `json:"lastRecordAt,omitempty"`
	RatePerMinHour float64   `json:"ratePerMinLastHour"`
	RatePerMin24h  float64   `json:"ratePerMinLast24h"`
}

type metricsAPI struct {
	runtime *connectors.Runtime
	store   storage.Storage
}

// GET /api/connectors/{id}/metrics
//
// Returns lifecycle + ingest stats for one instance. Useful for
// dashboards, the marketplace UI's per-tile metric, and external
// scrapers that want to track per-connector throughput.
func (m *metricsAPI) handle(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	st, exists := m.runtime.Status(id)
	out := InstanceMetrics{InstanceID: id}
	if exists {
		out.State = string(st.State)
		out.Restarts = st.Restarts
		out.StartedAt = st.StartedAt
		out.LastError = st.LastError
		out.LastErrorAt = st.LastErrorAt
	} else {
		out.State = "unknown"
	}

	// Total record count via the storage's aggregated counter — cheap.
	if counts, err := m.store.CountByConnector(r.Context()); err == nil {
		out.TotalRecords = counts[id]
	}

	// Last record timestamp.
	if rs, err := m.store.ByConnector(r.Context(), id, time.Time{}, time.Time{}, 1); err == nil && len(rs) > 0 {
		t := rs[0].Timestamp
		out.LastRecordAt = &t
	}

	// Rates: count buckets over the last hour and last 24h.
	now := time.Now().UTC()
	if buckets, err := m.store.Timeseries(r.Context(), id, now.Add(-time.Hour), now, time.Minute); err == nil {
		var sum int64
		for _, b := range buckets {
			sum += b.Count
		}
		out.RatePerMinHour = float64(sum) / 60.0
	}
	if buckets, err := m.store.Timeseries(r.Context(), id, now.Add(-24*time.Hour), now, time.Minute); err == nil {
		var sum int64
		for _, b := range buckets {
			sum += b.Count
		}
		out.RatePerMin24h = float64(sum) / (24.0 * 60.0)
	}

	writeJSON(w, http.StatusOK, out)
}
