package storage

import (
	"context"
	"net/url"
	"strings"
	"testing"
)

func TestOpenDSN_DuckDBMemory(t *testing.T) {
	t.Parallel()
	be, err := OpenDSN(context.Background(), ":memory:")
	if err != nil {
		t.Fatalf("OpenDSN: %v", err)
	}
	defer be.Close()
	if _, err := be.Recent(context.Background(), 1); err != nil {
		t.Fatalf("Recent on fresh backend: %v", err)
	}
}

func TestOpenDSN_DuckDBExplicitScheme(t *testing.T) {
	t.Parallel()
	be, err := OpenDSN(context.Background(), "duckdb://:memory:")
	if err != nil {
		t.Fatalf("OpenDSN: %v", err)
	}
	defer be.Close()
}

func TestOpenDSN_BarePathDefaultsToDuckDB(t *testing.T) {
	t.Parallel()
	tmp := t.TempDir() + "/db.duckdb"
	be, err := OpenDSN(context.Background(), tmp)
	if err != nil {
		t.Fatalf("OpenDSN: %v", err)
	}
	defer be.Close()
}

func TestOpenDSN_UnknownScheme(t *testing.T) {
	t.Parallel()
	_, err := OpenDSN(context.Background(), "made-up-scheme://foo")
	if err == nil {
		t.Fatalf("expected error for unknown scheme")
	}
	if !strings.Contains(err.Error(), "unknown scheme") {
		t.Fatalf("expected 'unknown scheme' in error, got %q", err.Error())
	}
}

func TestOpenDSN_EmptyDSN(t *testing.T) {
	t.Parallel()
	_, err := OpenDSN(context.Background(), "")
	if err == nil {
		t.Fatalf("expected error for empty DSN")
	}
}

func TestSchemes_IncludesDuckDB(t *testing.T) {
	t.Parallel()
	found := false
	for _, s := range Schemes() {
		if s == "duckdb" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("duckdb not in registered schemes: %v", Schemes())
	}
}

func TestRegister_PanicsOnDuplicate(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatalf("expected panic on duplicate Register of an existing scheme")
		}
	}()
	Register("duckdb", func(_ context.Context, _ *url.URL) (Backend, error) {
		return nil, nil
	})
}

func TestRegister_NewSchemeWorks(t *testing.T) {
	// Not parallel: mutates the global registry.
	scheme := "testfakeregisterscheme"
	Register(scheme, func(_ context.Context, _ *url.URL) (Backend, error) {
		return nil, nil
	})
	be, err := OpenDSN(context.Background(), scheme+"://anything")
	if err != nil {
		t.Fatalf("OpenDSN: %v", err)
	}
	if be != nil {
		t.Fatalf("expected nil backend from test constructor")
	}
}
