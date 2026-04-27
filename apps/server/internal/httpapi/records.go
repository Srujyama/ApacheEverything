package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/sunny/sunny/apps/server/internal/storage"
)

// recordsAPI exposes persisted records from storage.
type recordsAPI struct {
	store storage.Storage
}

// list returns persisted records, optionally filtered by connector and time.
//
//	GET /api/records?connector=<id>&from=<rfc3339>&to=<rfc3339>&limit=<n>
//
// If connector is omitted, returns the most recent N records across all
// connectors. With connector set, applies the time range and per-connector
// index for efficient lookup.
func (r *recordsAPI) list(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()
	connectorID := q.Get("connector")
	limit := parseIntParam(q.Get("limit"), 100, 1, 10000)

	from, err := parseTimeParam(q.Get("from"))
	if err != nil {
		http.Error(w, "invalid from: "+err.Error(), http.StatusBadRequest)
		return
	}
	to, err := parseTimeParam(q.Get("to"))
	if err != nil {
		http.Error(w, "invalid to: "+err.Error(), http.StatusBadRequest)
		return
	}

	ctx := req.Context()
	var (
		recs interface{}
		qerr error
	)
	if connectorID == "" {
		recs, qerr = r.store.Recent(ctx, limit)
	} else {
		recs, qerr = r.store.ByConnector(ctx, connectorID, from, to, limit)
	}
	if qerr != nil {
		http.Error(w, qerr.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, recs)
}

func parseTimeParam(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s)
}

func parseIntParam(s string, def, min, max int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	if n < min {
		return min
	}
	if n > max {
		return max
	}
	return n
}
