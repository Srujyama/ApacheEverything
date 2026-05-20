package main

import (
	"database/sql"
	"testing"
	"time"

	_ "github.com/marcboeker/go-duckdb/v2"
)

func TestMigrate_CopiesAllTables(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	src := tmp + "/src.duckdb"
	dst := tmp + "/dst.duckdb"

	// Seed src with the schema + a few rows.
	db, err := sql.Open("duckdb", src)
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`CREATE TABLE events (
			timestamp TIMESTAMPTZ NOT NULL,
			connector_id TEXT NOT NULL,
			source_id TEXT,
			lat DOUBLE, lng DOUBLE, alt DOUBLE,
			tags JSON, payload JSON
		)`,
		`CREATE TABLE checkpoints (
			instance_id TEXT NOT NULL, key TEXT NOT NULL,
			value TEXT NOT NULL, updated_at TIMESTAMPTZ NOT NULL,
			PRIMARY KEY (instance_id, key)
		)`,
		`CREATE TABLE alert_rules (
			id TEXT PRIMARY KEY, name TEXT NOT NULL, enabled BOOLEAN NOT NULL,
			connector_id TEXT, severity_in JSON, tag_equals JSON,
			created_at TIMESTAMPTZ NOT NULL
		)`,
		`CREATE TABLE alerts (
			id TEXT PRIMARY KEY, rule_id TEXT NOT NULL, rule_name TEXT NOT NULL,
			connector_id TEXT NOT NULL, source_id TEXT,
			severity TEXT, headline TEXT, tags JSON, payload JSON,
			triggered TIMESTAMPTZ NOT NULL, acked_at TIMESTAMPTZ
		)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("seed schema: %v", err)
		}
	}
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := db.Exec(`INSERT INTO events VALUES (?, ?, ?, NULL, NULL, NULL, '{}', '{}')`,
		now, "conn-1", "src-1"); err != nil {
		t.Fatalf("seed event: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO checkpoints VALUES (?, ?, ?, ?)`,
		"inst", "k", "v", now); err != nil {
		t.Fatalf("seed checkpoint: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO alert_rules VALUES (?, ?, TRUE, NULL, '[]', '{}', ?)`,
		"r1", "rule1", now); err != nil {
		t.Fatalf("seed rule: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO alerts VALUES (?, ?, ?, ?, NULL, ?, ?, '{}', '{}', ?, NULL)`,
		"a1", "r1", "rule1", "conn-1", "critical", "boom", now); err != nil {
		t.Fatalf("seed alert: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// Run migrate.
	if err := migrateCmd([]string{"--from", src, "--to", dst}); err != nil {
		t.Fatalf("migrateCmd: %v", err)
	}

	// Verify dst.
	dst2, err := sql.Open("duckdb", dst)
	if err != nil {
		t.Fatal(err)
	}
	defer dst2.Close()
	for _, tab := range []string{"events", "checkpoints", "alert_rules", "alerts"} {
		var n int
		if err := dst2.QueryRow("SELECT COUNT(*) FROM " + tab).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", tab, err)
		}
		if n != 1 {
			t.Errorf("table %s: got %d rows, want 1", tab, n)
		}
	}
}

func TestMigrate_RefusesExistingDest(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir()
	src := tmp + "/src.duckdb"
	dst := tmp + "/dst.duckdb"

	// Create both files.
	for _, p := range []string{src, dst} {
		db, err := sql.Open("duckdb", p)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = db.Exec("CREATE TABLE events (timestamp TIMESTAMPTZ NOT NULL, connector_id TEXT NOT NULL, source_id TEXT, lat DOUBLE, lng DOUBLE, alt DOUBLE, tags JSON, payload JSON)")
		_ = db.Close()
	}

	err := migrateCmd([]string{"--from", src, "--to", dst})
	if err == nil {
		t.Fatal("expected refusal when destination exists")
	}
}

func TestMigrate_RejectsSamePath(t *testing.T) {
	t.Parallel()
	err := migrateCmd([]string{"--from", "x.duckdb", "--to", "x.duckdb"})
	if err == nil {
		t.Fatal("expected error when --from == --to")
	}
}
