package httpapi

import (
	"encoding/json"
	"net/http"

	"github.com/sunny/sunny/apps/server/internal/storage"
)

type queryAPI struct{ store storage.Storage }

// POST /api/query
//
//	{
//	  "sql": "SELECT connector_id, COUNT(*) FROM events GROUP BY connector_id",
//	  "params": [],
//	  "limit": 1000
//	}
//
// Returns columnar results. SELECT and WITH only. Multi-statement and
// DDL/DML are rejected. The endpoint is auth-gated so untrusted callers
// can't hit it; even authenticated callers are capped at MaxQueryRows.
func (q *queryAPI) handle(w http.ResponseWriter, r *http.Request) {
	type body struct {
		SQL    string `json:"sql"`
		Params []any  `json:"params"`
		Limit  int    `json:"limit"`
	}
	var b body
	if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if b.SQL == "" {
		http.Error(w, "sql is required", http.StatusBadRequest)
		return
	}

	querier, ok := q.store.(storage.Querier)
	if !ok {
		http.Error(w, "this storage backend doesn't support ad-hoc queries", http.StatusInternalServerError)
		return
	}

	result, err := querier.Query(r.Context(), b.SQL, b.Params, b.Limit)
	if err != nil {
		// Treat allowlist failures as 400, runtime errors as 500. The
		// allowlist messages all start with their own static prefixes.
		switch err.Error() {
		case "empty query",
			"multi-statement queries are not allowed",
			"only SELECT and WITH are allowed",
			"query contains a forbidden keyword":
			http.Error(w, err.Error(), http.StatusBadRequest)
		default:
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
		return
	}
	writeJSON(w, http.StatusOK, result)
}
