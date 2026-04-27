package storage

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	sdk "github.com/sunny/sunny/packages/sdk-go"
)

func TestQueryAllowsSelect(t *testing.T) {
	s := newTestStore(t).(*duckStorage)
	ctx := context.Background()
	now := time.Now().UTC()
	if err := s.Write(ctx, []sdk.Record{
		{Timestamp: now, ConnectorID: "a", Payload: json.RawMessage(`{}`)},
		{Timestamp: now, ConnectorID: "b", Payload: json.RawMessage(`{}`)},
		{Timestamp: now, ConnectorID: "a", Payload: json.RawMessage(`{}`)},
	}); err != nil {
		t.Fatal(err)
	}

	r, err := s.Query(ctx,
		`SELECT connector_id, COUNT(*) AS n FROM events GROUP BY connector_id ORDER BY connector_id`,
		nil, 0)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(r.Columns) != 2 || r.Columns[0] != "connector_id" {
		t.Fatalf("columns = %v", r.Columns)
	}
	if r.Rows64 != 2 {
		t.Fatalf("rows = %d, want 2", r.Rows64)
	}
}

func TestQueryAllowsCTE(t *testing.T) {
	s := newTestStore(t).(*duckStorage)
	r, err := s.Query(context.Background(),
		`WITH x AS (SELECT 1 AS n) SELECT n FROM x`, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	if r.Rows64 != 1 {
		t.Fatalf("rows = %d", r.Rows64)
	}
}

func TestQueryRejectsMultiStatement(t *testing.T) {
	s := newTestStore(t).(*duckStorage)
	_, err := s.Query(context.Background(),
		`SELECT 1; SELECT 2`, nil, 0)
	if err == nil {
		t.Fatal("expected multi-statement rejection")
	}
}

func TestQueryAllowsTrailingSemicolon(t *testing.T) {
	s := newTestStore(t).(*duckStorage)
	_, err := s.Query(context.Background(),
		`SELECT 1;`, nil, 0)
	if err != nil {
		t.Fatalf("trailing semicolon should be allowed: %v", err)
	}
}

func TestQueryRejectsInsert(t *testing.T) {
	s := newTestStore(t).(*duckStorage)
	cases := []string{
		`INSERT INTO events VALUES ('','','','','','')`,
		`UPDATE events SET source_id = 'x'`,
		`DELETE FROM events`,
		`DROP TABLE events`,
		`CREATE TABLE x (a INT)`,
		`COPY events TO 'x.csv'`,
		`PRAGMA foo`,
	}
	for _, q := range cases {
		_, err := s.Query(context.Background(), q, nil, 0)
		if err == nil {
			t.Fatalf("expected rejection for %q", q)
		}
	}
}

func TestQueryRejectsHiddenInsert(t *testing.T) {
	s := newTestStore(t).(*duckStorage)
	// User tries to disguise an insert by chaining onto a SELECT.
	_, err := s.Query(context.Background(),
		`SELECT 1 UNION SELECT 1; INSERT INTO events VALUES ('','','','','','')`,
		nil, 0)
	if err == nil {
		t.Fatal("expected rejection")
	}
}

func TestQueryEnforcesLimit(t *testing.T) {
	s := newTestStore(t).(*duckStorage)
	ctx := context.Background()
	for i := 0; i < 50; i++ {
		_ = s.Write(ctx, []sdk.Record{
			{Timestamp: time.Now(), ConnectorID: "c", Payload: json.RawMessage(`{}`)},
		})
	}
	r, err := s.Query(ctx, `SELECT * FROM events`, nil, 5)
	if err != nil {
		t.Fatal(err)
	}
	if r.Rows64 != 5 {
		t.Fatalf("rows = %d, want 5", r.Rows64)
	}
}

func TestQueryParameterized(t *testing.T) {
	s := newTestStore(t).(*duckStorage)
	ctx := context.Background()
	now := time.Now().UTC()
	if err := s.Write(ctx, []sdk.Record{
		{Timestamp: now, ConnectorID: "param-a", Payload: json.RawMessage(`{}`)},
		{Timestamp: now, ConnectorID: "param-b", Payload: json.RawMessage(`{}`)},
	}); err != nil {
		t.Fatal(err)
	}
	r, err := s.Query(ctx, `SELECT connector_id FROM events WHERE connector_id = ?`,
		[]any{"param-a"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if r.Rows64 != 1 {
		t.Fatalf("rows = %d, want 1", r.Rows64)
	}
	if cid, ok := r.Rows[0][0].(string); !ok || !strings.Contains(cid, "param-a") {
		t.Fatalf("first row = %v", r.Rows[0])
	}
}

func TestQueryEmptyRejected(t *testing.T) {
	s := newTestStore(t).(*duckStorage)
	_, err := s.Query(context.Background(), "", nil, 0)
	if err == nil {
		t.Fatal("expected error for empty query")
	}
	_, err = s.Query(context.Background(), "   ", nil, 0)
	if err == nil {
		t.Fatal("expected error for whitespace-only query")
	}
}
