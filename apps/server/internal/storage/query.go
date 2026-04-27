package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// QueryResult is the shape returned by Query.
type QueryResult struct {
	Columns []string         `json:"columns"`
	Rows    [][]any          `json:"rows"`
	Rows64  int64            `json:"rowCount"`
	Types   []string         `json:"types,omitempty"`
}

// Querier exposes safe ad-hoc reads over the events table.
type Querier interface {
	Query(ctx context.Context, sqlText string, params []any, limit int) (*QueryResult, error)
}

// queryAllowedRE matches statements that begin with SELECT or WITH (CTE).
// The lazy prefix-match is intentional — we re-check below for forbidden
// statement separators too.
var queryAllowedRE = regexp.MustCompile(`(?is)^\s*(select|with)\b`)

// queryForbiddenRE catches multi-statement attempts and DDL/DML keywords
// that even a SELECT query shouldn't contain. Anchored search across the
// whole text.
var queryForbiddenRE = regexp.MustCompile(`(?is)\b(insert|update|delete|drop|alter|create|truncate|grant|revoke|copy|attach|pragma|export|import|set|call|use|reset|vacuum|analyze)\b`)

// MaxQueryRows caps how many rows a single /api/query call can return.
// Exists so a user accidentally doing `SELECT * FROM events` doesn't OOM
// a small VPS. Configurable via the limit parameter, but capped here.
const MaxQueryRows = 10_000

// Query runs a read-only SELECT (or WITH...SELECT) and returns columnar
// results. Multi-statement input is rejected; so are DDL/DML keywords.
//
// It's not a sandbox — DuckDB runs the statement against the live `events`
// table and any view in scope. Don't expose this endpoint to the public
// internet without auth.
func (s *duckStorage) Query(ctx context.Context, sqlText string, params []any, limit int) (*QueryResult, error) {
	q := strings.TrimSpace(sqlText)
	if q == "" {
		return nil, errors.New("empty query")
	}
	// Strip a single trailing semicolon so users can paste from psql.
	q = strings.TrimRight(q, ";")
	if strings.Contains(q, ";") {
		return nil, errors.New("multi-statement queries are not allowed")
	}
	if !queryAllowedRE.MatchString(q) {
		return nil, errors.New("only SELECT and WITH are allowed")
	}
	if queryForbiddenRE.MatchString(q) {
		return nil, errors.New("query contains a forbidden keyword")
	}

	if limit <= 0 || limit > MaxQueryRows {
		limit = MaxQueryRows
	}

	// Wrap the user's query in a LIMIT so even a SELECT * is bounded.
	wrapped := fmt.Sprintf("SELECT * FROM (%s) AS sunny_query LIMIT %d", q, limit)

	rows, err := s.db.QueryContext(ctx, wrapped, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	typeNames := make([]string, len(colTypes))
	for i, ct := range colTypes {
		typeNames[i] = ct.DatabaseTypeName()
	}

	out := &QueryResult{Columns: cols, Types: typeNames}
	for rows.Next() {
		raw := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range raw {
			ptrs[i] = &raw[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		// Normalize a few common types so JSON encoding is sensible.
		row := make([]any, len(cols))
		for i, v := range raw {
			row[i] = normalizeValue(v)
		}
		out.Rows = append(out.Rows, row)
		out.Rows64++
	}
	return out, rows.Err()
}

func normalizeValue(v any) any {
	switch x := v.(type) {
	case []byte:
		return string(x)
	case sql.NullString:
		if x.Valid {
			return x.String
		}
		return nil
	case sql.NullInt64:
		if x.Valid {
			return x.Int64
		}
		return nil
	case sql.NullFloat64:
		if x.Valid {
			return x.Float64
		}
		return nil
	default:
		return v
	}
}
