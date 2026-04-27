package httpapi

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/sunny/sunny/apps/server/internal/storage"
)

type exportAPI struct{ store storage.Storage }

// GET /api/export?format=csv|parquet&connector=&from=&to=&limit=
//
// Streams the matching events. Parquet exports buffer to a tempfile
// (Parquet's not streamable). CSV exports stream directly.
func (e *exportAPI) handle(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	format := strings.ToLower(strings.TrimSpace(q.Get("format")))
	if format == "" {
		format = "csv"
	}
	var ef storage.ExportFormat
	switch format {
	case "csv":
		ef = storage.ExportCSV
	case "parquet":
		ef = storage.ExportParquet
	default:
		http.Error(w, "format must be csv or parquet", http.StatusBadRequest)
		return
	}

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

	limit := int64(parseIntParam(q.Get("limit"), 100000, 1, 1_000_000))

	exporter, ok := e.store.(storage.Exporter)
	if !ok {
		http.Error(w, "exporter not supported by this storage backend", http.StatusInternalServerError)
		return
	}

	filter := storage.ExportFilter{
		ConnectorID: q.Get("connector"),
		From:        from,
		To:          to,
		Limit:       limit,
	}

	switch ef {
	case storage.ExportCSV:
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="sunny-export.csv"`)
	case storage.ExportParquet:
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Content-Disposition", `attachment; filename="sunny-export.parquet"`)
	}

	rows, err := exporter.Export(r.Context(), w, ef, filter)
	if err != nil {
		// Headers already sent — best we can do is log. Callers will see a
		// truncated download.
		_ = strconv.AppendInt(nil, rows, 10) // suppress unused warning
		return
	}
}
