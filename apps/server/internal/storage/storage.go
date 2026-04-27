// Package storage is Sunny's persistence layer.
//
// Phase 2 ships a single embedded DuckDB implementation. The Storage
// interface is intentionally narrow so phase 2.5 can drop in a ClickHouse
// implementation without touching connectors, runtime, or HTTP handlers.
//
// Schema (single wide events table):
//
//	timestamp     TIMESTAMPTZ      -- when the observation happened
//	connector_id  TEXT             -- e.g. "hello-1" (instance id)
//	source_id     TEXT             -- e.g. sensor or asset id, may be ""
//	lat,lng,alt   DOUBLE           -- nullable WGS84
//	tags          JSON             -- flat string->string
//	payload       JSON             -- arbitrary connector payload
//
// We deliberately don't shred payload into columns. Connectors evolve faster
// than schemas, and DuckDB's JSON functions are fast enough for the query
// patterns we need (filter by connector + time range, then drill into payload).
package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	_ "github.com/marcboeker/go-duckdb/v2"

	sdk "github.com/sunny/sunny/packages/sdk-go"
)

// Storage is the contract every storage backend implements.
type Storage interface {
	// Write persists a batch of records. Empty slice is a no-op.
	Write(ctx context.Context, records []sdk.Record) error

	// Recent returns the latest n records across all connectors, newest first.
	Recent(ctx context.Context, limit int) ([]sdk.Record, error)

	// ByConnector returns records for one connector, optionally bounded by
	// time. Zero-value times mean "unbounded" on that side.
	ByConnector(ctx context.Context, connectorID string, from, to time.Time, limit int) ([]sdk.Record, error)

	// SaveCheckpoint and LoadCheckpoint persist small strings keyed by
	// (instanceID, key). Used by pull connectors to resume after a restart.
	SaveCheckpoint(ctx context.Context, instanceID, key, value string) error
	LoadCheckpoint(ctx context.Context, instanceID, key string) (string, error)

	// CountByConnector returns the total record count grouped by connector_id.
	CountByConnector(ctx context.Context) (map[string]int64, error)

	// Timeseries returns time-bucketed record counts. If connectorID is empty,
	// counts across all connectors. bucket must be a non-zero positive duration.
	Timeseries(ctx context.Context, connectorID string, from, to time.Time, bucket time.Duration) ([]TimeseriesBucket, error)

	// Alerts: rules + triggered alerts.
	SaveRule(ctx context.Context, r AlertRule) error
	DeleteRule(ctx context.Context, id string) error
	ListRules(ctx context.Context) ([]AlertRule, error)
	InsertAlert(ctx context.Context, a Alert) error
	ListAlerts(ctx context.Context, limit int) ([]Alert, error)
	AckAlert(ctx context.Context, id string, at time.Time) error

	// Close releases resources.
	Close() error
}

// TimeseriesBucket is one bucket of the Timeseries result.
type TimeseriesBucket struct {
	Bucket time.Time `json:"bucket"`
	Count  int64     `json:"count"`
}

// AlertRule defines what triggers an alert.
type AlertRule struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Enabled     bool              `json:"enabled"`
	ConnectorID string            `json:"connectorId,omitempty"` // empty = any
	SeverityIn  []string          `json:"severityIn,omitempty"`  // tags.severity match
	TagEquals   map[string]string `json:"tagEquals,omitempty"`   // all-must-match
	CreatedAt   time.Time         `json:"createdAt"`
}

// Alert is a triggered evaluation of an AlertRule against a Record.
type Alert struct {
	ID          string            `json:"id"`
	RuleID      string            `json:"ruleId"`
	RuleName    string            `json:"ruleName"`
	ConnectorID string            `json:"connectorId"`
	SourceID    string            `json:"sourceId,omitempty"`
	Severity    string            `json:"severity"`
	Headline    string            `json:"headline"`
	Tags        map[string]string `json:"tags,omitempty"`
	Payload     json.RawMessage   `json:"payload,omitempty"`
	Triggered   time.Time         `json:"triggered"`
	Acked       *time.Time        `json:"acked,omitempty"`
}

// Open returns a Storage backed by an embedded DuckDB at path.
// Pass ":memory:" for an in-memory DB (useful for tests).
func Open(path string) (Storage, error) {
	db, err := sql.Open("duckdb", path)
	if err != nil {
		return nil, fmt.Errorf("open duckdb: %w", err)
	}
	// DuckDB is single-writer-friendly; conservative pool keeps things sane.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)

	s := &duckStorage{db: db}
	if err := s.migrate(context.Background()); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}
	return s, nil
}

type duckStorage struct {
	db *sql.DB
}

func (s *duckStorage) migrate(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS events (
			timestamp     TIMESTAMPTZ NOT NULL,
			connector_id  TEXT        NOT NULL,
			source_id     TEXT,
			lat           DOUBLE,
			lng           DOUBLE,
			alt           DOUBLE,
			tags          JSON,
			payload       JSON
		)`,
		`CREATE INDEX IF NOT EXISTS idx_events_connector_ts ON events (connector_id, timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_events_ts ON events (timestamp DESC)`,
		`CREATE TABLE IF NOT EXISTS checkpoints (
			instance_id TEXT NOT NULL,
			key         TEXT NOT NULL,
			value       TEXT NOT NULL,
			updated_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (instance_id, key)
		)`,
		`CREATE TABLE IF NOT EXISTS alert_rules (
			id           TEXT PRIMARY KEY,
			name         TEXT NOT NULL,
			enabled      BOOLEAN NOT NULL DEFAULT TRUE,
			connector_id TEXT,
			severity_in  JSON,
			tag_equals   JSON,
			created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
		)`,
		`CREATE TABLE IF NOT EXISTS alerts (
			id           TEXT PRIMARY KEY,
			rule_id      TEXT NOT NULL,
			rule_name    TEXT NOT NULL,
			connector_id TEXT NOT NULL,
			source_id    TEXT,
			severity     TEXT,
			headline     TEXT,
			tags         JSON,
			payload      JSON,
			triggered    TIMESTAMPTZ NOT NULL,
			acked_at     TIMESTAMPTZ
		)`,
		`CREATE INDEX IF NOT EXISTS idx_alerts_triggered ON alerts (triggered DESC)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate %q: %w", stmt[:min(60, len(stmt))], err)
		}
	}
	return nil
}

func (s *duckStorage) Write(ctx context.Context, records []sdk.Record) error {
	if len(records) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO events (timestamp, connector_id, source_id, lat, lng, alt, tags, payload)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range records {
		var lat, lng, alt sql.NullFloat64
		if r.Location != nil {
			lat = sql.NullFloat64{Float64: r.Location.Lat, Valid: true}
			lng = sql.NullFloat64{Float64: r.Location.Lng, Valid: true}
			if r.Location.Altitude != nil {
				alt = sql.NullFloat64{Float64: *r.Location.Altitude, Valid: true}
			}
		}

		var tags any
		if len(r.Tags) > 0 {
			b, err := json.Marshal(r.Tags)
			if err != nil {
				return fmt.Errorf("marshal tags: %w", err)
			}
			tags = string(b)
		}

		payload := "{}"
		if len(r.Payload) > 0 {
			payload = string(r.Payload)
		}

		ts := r.Timestamp
		if ts.IsZero() {
			ts = time.Now().UTC()
		}

		if _, err := stmt.ExecContext(ctx,
			ts, r.ConnectorID, nullStr(r.SourceID),
			lat, lng, alt, tags, payload,
		); err != nil {
			return fmt.Errorf("insert event: %w", err)
		}
	}
	return tx.Commit()
}

func (s *duckStorage) Recent(ctx context.Context, limit int) ([]sdk.Record, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT timestamp, connector_id, source_id, lat, lng, alt, CAST(tags AS VARCHAR), CAST(payload AS VARCHAR)
		FROM events
		ORDER BY timestamp DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRecords(rows)
}

func (s *duckStorage) ByConnector(ctx context.Context, connectorID string, from, to time.Time, limit int) ([]sdk.Record, error) {
	if limit <= 0 {
		limit = 1000
	}
	q := `SELECT timestamp, connector_id, source_id, lat, lng, alt, CAST(tags AS VARCHAR), CAST(payload AS VARCHAR)
	      FROM events WHERE connector_id = ?`
	args := []any{connectorID}
	if !from.IsZero() {
		q += " AND timestamp >= ?"
		args = append(args, from)
	}
	if !to.IsZero() {
		q += " AND timestamp < ?"
		args = append(args, to)
	}
	q += " ORDER BY timestamp DESC LIMIT ?"
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRecords(rows)
}

func (s *duckStorage) SaveCheckpoint(ctx context.Context, instanceID, key, value string) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO checkpoints (instance_id, key, value, updated_at)
		VALUES (?, ?, ?, now())
		ON CONFLICT (instance_id, key) DO UPDATE SET value = excluded.value, updated_at = now()
	`, instanceID, key, value)
	return err
}

func (s *duckStorage) LoadCheckpoint(ctx context.Context, instanceID, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx,
		`SELECT value FROM checkpoints WHERE instance_id = ? AND key = ?`,
		instanceID, key,
	).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func (s *duckStorage) Close() error { return s.db.Close() }

// --- helpers ---

func scanRecords(rows *sql.Rows) ([]sdk.Record, error) {
	var out []sdk.Record
	for rows.Next() {
		var (
			ts          time.Time
			connectorID string
			sourceID    sql.NullString
			lat, lng    sql.NullFloat64
			alt         sql.NullFloat64
			tagsJSON    sql.NullString
			payloadJSON sql.NullString
		)
		if err := rows.Scan(&ts, &connectorID, &sourceID, &lat, &lng, &alt, &tagsJSON, &payloadJSON); err != nil {
			return nil, err
		}
		r := sdk.Record{
			Timestamp:   ts,
			ConnectorID: connectorID,
		}
		if sourceID.Valid {
			r.SourceID = sourceID.String
		}
		if lat.Valid && lng.Valid {
			r.Location = &sdk.GeoPoint{Lat: lat.Float64, Lng: lng.Float64}
			if alt.Valid {
				a := alt.Float64
				r.Location.Altitude = &a
			}
		}
		if tagsJSON.Valid && tagsJSON.String != "" {
			tags := map[string]string{}
			if err := json.Unmarshal([]byte(tagsJSON.String), &tags); err == nil {
				r.Tags = tags
			}
		}
		if payloadJSON.Valid && payloadJSON.String != "" {
			r.Payload = json.RawMessage(payloadJSON.String)
		} else {
			r.Payload = json.RawMessage("{}")
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func nullStr(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}

// --- counts / timeseries ---

func (s *duckStorage) CountByConnector(ctx context.Context) (map[string]int64, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT connector_id, COUNT(*) FROM events GROUP BY connector_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var k string
		var n int64
		if err := rows.Scan(&k, &n); err != nil {
			return nil, err
		}
		out[k] = n
	}
	return out, rows.Err()
}

func (s *duckStorage) Timeseries(ctx context.Context, connectorID string, from, to time.Time, bucket time.Duration) ([]TimeseriesBucket, error) {
	if bucket <= 0 {
		bucket = time.Minute
	}
	// DuckDB time_bucket with INTERVAL parameter; we use seconds for simplicity.
	q := `SELECT time_bucket(INTERVAL '1 second' * ?, timestamp) AS b, COUNT(*) AS c
	      FROM events WHERE 1=1`
	args := []any{int64(bucket.Seconds())}
	if connectorID != "" {
		q += " AND connector_id = ?"
		args = append(args, connectorID)
	}
	if !from.IsZero() {
		q += " AND timestamp >= ?"
		args = append(args, from)
	}
	if !to.IsZero() {
		q += " AND timestamp < ?"
		args = append(args, to)
	}
	q += " GROUP BY b ORDER BY b"

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TimeseriesBucket
	for rows.Next() {
		var b time.Time
		var c int64
		if err := rows.Scan(&b, &c); err != nil {
			return nil, err
		}
		out = append(out, TimeseriesBucket{Bucket: b.UTC(), Count: c})
	}
	return out, rows.Err()
}

// --- alert rules ---

func (s *duckStorage) SaveRule(ctx context.Context, r AlertRule) error {
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now().UTC()
	}
	sevJSON, _ := json.Marshal(r.SeverityIn)
	tagJSON, _ := json.Marshal(r.TagEquals)
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO alert_rules (id, name, enabled, connector_id, severity_in, tag_equals, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO UPDATE SET
			name = excluded.name,
			enabled = excluded.enabled,
			connector_id = excluded.connector_id,
			severity_in = excluded.severity_in,
			tag_equals = excluded.tag_equals
	`, r.ID, r.Name, r.Enabled, nullStr(r.ConnectorID), string(sevJSON), string(tagJSON), r.CreatedAt)
	return err
}

func (s *duckStorage) DeleteRule(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM alert_rules WHERE id = ?`, id)
	return err
}

func (s *duckStorage) ListRules(ctx context.Context) ([]AlertRule, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, enabled, connector_id, CAST(severity_in AS VARCHAR), CAST(tag_equals AS VARCHAR), created_at
		FROM alert_rules ORDER BY created_at
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AlertRule
	for rows.Next() {
		var (
			r       AlertRule
			conn    sql.NullString
			sevJSON sql.NullString
			tagJSON sql.NullString
		)
		if err := rows.Scan(&r.ID, &r.Name, &r.Enabled, &conn, &sevJSON, &tagJSON, &r.CreatedAt); err != nil {
			return nil, err
		}
		if conn.Valid {
			r.ConnectorID = conn.String
		}
		if sevJSON.Valid && sevJSON.String != "" && sevJSON.String != "null" {
			_ = json.Unmarshal([]byte(sevJSON.String), &r.SeverityIn)
		}
		if tagJSON.Valid && tagJSON.String != "" && tagJSON.String != "null" {
			_ = json.Unmarshal([]byte(tagJSON.String), &r.TagEquals)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// --- alerts ---

func (s *duckStorage) InsertAlert(ctx context.Context, a Alert) error {
	tagJSON, _ := json.Marshal(a.Tags)
	payloadStr := "{}"
	if len(a.Payload) > 0 {
		payloadStr = string(a.Payload)
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO alerts (id, rule_id, rule_name, connector_id, source_id, severity, headline, tags, payload, triggered, acked_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NULL)
	`, a.ID, a.RuleID, a.RuleName, a.ConnectorID, nullStr(a.SourceID),
		nullStr(a.Severity), nullStr(a.Headline), string(tagJSON), payloadStr, a.Triggered)
	return err
}

func (s *duckStorage) ListAlerts(ctx context.Context, limit int) ([]Alert, error) {
	if limit <= 0 {
		limit = 200
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, rule_id, rule_name, connector_id, source_id, severity, headline,
		       CAST(tags AS VARCHAR), CAST(payload AS VARCHAR), triggered, acked_at
		FROM alerts ORDER BY triggered DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Alert
	for rows.Next() {
		var (
			a        Alert
			src, sev sql.NullString
			hl       sql.NullString
			tagJSON  sql.NullString
			payload  sql.NullString
			acked    sql.NullTime
		)
		if err := rows.Scan(&a.ID, &a.RuleID, &a.RuleName, &a.ConnectorID, &src, &sev, &hl,
			&tagJSON, &payload, &a.Triggered, &acked); err != nil {
			return nil, err
		}
		if src.Valid {
			a.SourceID = src.String
		}
		if sev.Valid {
			a.Severity = sev.String
		}
		if hl.Valid {
			a.Headline = hl.String
		}
		if tagJSON.Valid && tagJSON.String != "" && tagJSON.String != "null" {
			_ = json.Unmarshal([]byte(tagJSON.String), &a.Tags)
		}
		if payload.Valid && payload.String != "" {
			a.Payload = json.RawMessage(payload.String)
		}
		if acked.Valid {
			t := acked.Time
			a.Acked = &t
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *duckStorage) AckAlert(ctx context.Context, id string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE alerts SET acked_at = ? WHERE id = ?`, at, id)
	return err
}
