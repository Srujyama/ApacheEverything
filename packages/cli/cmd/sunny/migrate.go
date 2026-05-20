// migrate.go implements `sunny-cli migrate`, a tool to move data between
// storage backends.
//
// v0.1 scope: copy `events`, `checkpoints`, `alert_rules`, and `alerts` from
// one DuckDB file to another. The DSN-based form (e.g. iceberg://...) lights
// up once Phase 1 ships the Iceberg backend; until then, both --from and
// --to must be DuckDB paths.
//
// Usage:
//
//   sunny-cli migrate --from ./old.duckdb --to ./new.duckdb
//
// The migration is offline: do NOT run it against a database that's still
// open by the live `sunny` server.
package main

import (
	"database/sql"
	"errors"
	"fmt"
	"os"

	_ "github.com/marcboeker/go-duckdb/v2"
)

func migrateCmd(args []string) error {
	from := ""
	to := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--from":
			if i+1 >= len(args) {
				return errors.New("--from needs a value")
			}
			from = args[i+1]; i++
		case "--to":
			if i+1 >= len(args) {
				return errors.New("--to needs a value")
			}
			to = args[i+1]; i++
		default:
			return fmt.Errorf("unexpected arg: %s", args[i])
		}
	}
	if from == "" || to == "" {
		return errors.New("usage: sunny-cli migrate --from <src.duckdb> --to <dst.duckdb>")
	}
	if from == to {
		return errors.New("--from and --to must differ")
	}
	if _, err := os.Stat(to); err == nil {
		return fmt.Errorf("destination %s already exists; refusing to overwrite", to)
	}

	src, err := sql.Open("duckdb", from)
	if err != nil {
		return fmt.Errorf("open src: %w", err)
	}
	defer src.Close()
	dst, err := sql.Open("duckdb", to)
	if err != nil {
		return fmt.Errorf("open dst: %w", err)
	}
	defer dst.Close()

	// Replicate schema (idempotent CREATEs). Mirrors apps/server/internal/storage/storage.go.
	schema := []string{
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
	}
	for _, stmt := range schema {
		if _, err := dst.Exec(stmt); err != nil {
			return fmt.Errorf("dst schema: %w", err)
		}
	}

	// Copy each table. JSON columns are CAST to VARCHAR in SELECT so they
	// can be re-bound; the dst column type accepts strings as JSON.
	tables := []struct {
		name      string
		cols      string // INSERT column list
		selectSQL string // SELECT expression list
	}{
		{
			"events",
			"timestamp, connector_id, source_id, lat, lng, alt, tags, payload",
			"timestamp, connector_id, source_id, lat, lng, alt, CAST(tags AS VARCHAR), CAST(payload AS VARCHAR)",
		},
		{
			"checkpoints",
			"instance_id, key, value, updated_at",
			"instance_id, key, value, updated_at",
		},
		{
			"alert_rules",
			"id, name, enabled, connector_id, severity_in, tag_equals, created_at",
			"id, name, enabled, connector_id, CAST(severity_in AS VARCHAR), CAST(tag_equals AS VARCHAR), created_at",
		},
		{
			"alerts",
			"id, rule_id, rule_name, connector_id, source_id, severity, headline, tags, payload, triggered, acked_at",
			"id, rule_id, rule_name, connector_id, source_id, severity, headline, CAST(tags AS VARCHAR), CAST(payload AS VARCHAR), triggered, acked_at",
		},
	}
	for _, t := range tables {
		n, err := copyTable(src, dst, t.name, t.cols, t.selectSQL)
		if err != nil {
			return fmt.Errorf("copy %s: %w", t.name, err)
		}
		fmt.Printf("  %s: %d rows\n", t.name, n)
	}
	fmt.Printf("Migration complete: %s → %s\n", from, to)
	return nil
}

// copyTable streams rows from src.<name> into dst.<name> using positional
// inserts on the given column list. selectExpr lets the caller cast
// JSON-typed columns to VARCHAR for safe rebinding.
func copyTable(src, dst *sql.DB, name, cols, selectExpr string) (int, error) {
	rows, err := src.Query("SELECT " + selectExpr + " FROM " + name)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	colCount := 0
	for _, c := range cols {
		if c == ',' {
			colCount++
		}
	}
	colCount++

	placeholders := ""
	for i := 0; i < colCount; i++ {
		if i > 0 {
			placeholders += ", "
		}
		placeholders += "?"
	}
	insertSQL := "INSERT INTO " + name + " (" + cols + ") VALUES (" + placeholders + ")"

	tx, err := dst.Begin()
	if err != nil {
		return 0, err
	}
	stmt, err := tx.Prepare(insertSQL)
	if err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	defer stmt.Close()

	count := 0
	for rows.Next() {
		values := make([]any, colCount)
		valuePtrs := make([]any, colCount)
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		if _, err := stmt.Exec(values...); err != nil {
			_ = tx.Rollback()
			return 0, err
		}
		count++
	}
	if err := rows.Err(); err != nil {
		_ = tx.Rollback()
		return 0, err
	}
	return count, tx.Commit()
}
