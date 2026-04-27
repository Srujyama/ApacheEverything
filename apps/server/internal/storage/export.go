package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// ExportFormat is what the user can export to. CSV is streamable; Parquet
// is buffered to a tempfile and then copied out.
type ExportFormat string

const (
	ExportCSV     ExportFormat = "csv"
	ExportParquet ExportFormat = "parquet"
)

// ExportFilter scopes the export.
type ExportFilter struct {
	ConnectorID string
	From, To    time.Time
	Limit       int64 // hard cap; 0 = no extra cap (storage-side LIMIT may still apply)
}

// Exporter is implemented by storage backends that can export query results.
type Exporter interface {
	Export(ctx context.Context, w io.Writer, format ExportFormat, filter ExportFilter) (rows int64, err error)
}

// Export streams a SELECT * over events to w in the requested format.
// CSV is written incrementally; Parquet uses a tempfile and is copied out.
func (s *duckStorage) Export(ctx context.Context, w io.Writer, format ExportFormat, f ExportFilter) (int64, error) {
	if format != ExportCSV && format != ExportParquet {
		return 0, fmt.Errorf("unsupported export format %q", format)
	}

	tmp, err := os.CreateTemp("", "sunny-export-*."+string(format))
	if err != nil {
		return 0, err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(tmpPath)

	where := "1=1"
	args := []any{}
	if f.ConnectorID != "" {
		where += " AND connector_id = ?"
		args = append(args, f.ConnectorID)
	}
	if !f.From.IsZero() {
		where += " AND timestamp >= ?"
		args = append(args, f.From)
	}
	if !f.To.IsZero() {
		where += " AND timestamp < ?"
		args = append(args, f.To)
	}
	limit := ""
	if f.Limit > 0 {
		limit = fmt.Sprintf(" LIMIT %d", f.Limit)
	}

	formatClause := ""
	switch format {
	case ExportCSV:
		formatClause = "(FORMAT CSV, HEADER)"
	case ExportParquet:
		formatClause = "(FORMAT PARQUET)"
	}

	q := fmt.Sprintf(`COPY (
		SELECT timestamp, connector_id, source_id, lat, lng, alt,
		       CAST(tags AS VARCHAR) AS tags,
		       CAST(payload AS VARCHAR) AS payload
		FROM events WHERE %s ORDER BY timestamp%s
	) TO '%s' %s`, where, limit, tmpPath, formatClause)

	if _, err := s.db.ExecContext(ctx, q, args...); err != nil {
		return 0, fmt.Errorf("copy to file: %w", err)
	}

	stat, err := os.Stat(tmpPath)
	if err != nil {
		return 0, err
	}
	in, err := os.Open(tmpPath)
	if err != nil {
		return 0, err
	}
	defer in.Close()

	n, err := io.Copy(w, in)
	if err != nil {
		return n, err
	}
	_ = stat
	// Row count: do a separate COUNT(*) so callers get something useful in
	// the response header. Cheap because it shares the predicate.
	row := s.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM events WHERE "+where+limit, args...,
	)
	var rows int64
	_ = row.Scan(&rows)
	return rows, nil
}

// ensure tempdir exists for export — pure helper, used in tests.
func ensureDir(dir string) error {
	if dir == "" {
		return nil
	}
	return os.MkdirAll(filepath.Clean(dir), 0o755)
}
