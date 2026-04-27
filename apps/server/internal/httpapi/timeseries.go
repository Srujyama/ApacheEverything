package httpapi

import (
	"net/http"
	"time"

	"github.com/sunny/sunny/apps/server/internal/storage"
)

type timeseriesAPI struct{ store storage.Storage }

// GET /api/timeseries?connector=&from=&to=&bucket=<seconds>
//
// Default window is the last hour, default bucket is 60 seconds.
func (t *timeseriesAPI) timeseries(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	connector := q.Get("connector")

	from, err := parseTimeParam(q.Get("from"))
	if err != nil {
		http.Error(w, "invalid from", http.StatusBadRequest)
		return
	}
	to, err := parseTimeParam(q.Get("to"))
	if err != nil {
		http.Error(w, "invalid to", http.StatusBadRequest)
		return
	}
	if from.IsZero() && to.IsZero() {
		to = time.Now().UTC()
		from = to.Add(-time.Hour)
	}

	bucketSec := parseIntParam(q.Get("bucket"), 60, 1, 86400)
	bucket := time.Duration(bucketSec) * time.Second

	out, err := t.store.Timeseries(r.Context(), connector, from, to, bucket)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if out == nil {
		out = []storage.TimeseriesBucket{}
	}
	writeJSON(w, http.StatusOK, out)
}

// GET /api/records/counts → map[connector]int64
func (t *timeseriesAPI) counts(w http.ResponseWriter, r *http.Request) {
	out, err := t.store.CountByConnector(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if out == nil {
		out = map[string]int64{}
	}
	writeJSON(w, http.StatusOK, out)
}
