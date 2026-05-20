// backend.go declares the composable interfaces that make up the Storage
// contract, plus a registry of backend constructors keyed by URL scheme.
//
// Why split: the v0.1 Storage interface lumped records, checkpoints, alerts,
// and rules together. Phase 1 (Iceberg/Delta on object storage) and Phase 2
// (Iceberg REST Catalog) need to compose backends — a lakehouse backend may
// hand records to Iceberg but keep checkpoints in a tiny metadata store.
// Splitting the interface lets us mix-and-match without rewriting callers.
//
// The full Storage interface in storage.go still exists for backwards
// compatibility; it just embeds these smaller interfaces.

package storage

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"sync"
	"time"

	sdk "github.com/sunny/sunny/packages/sdk-go"
)

// RecordStore persists and reads ingest records.
type RecordStore interface {
	Write(ctx context.Context, records []sdk.Record) error
	Recent(ctx context.Context, limit int) ([]sdk.Record, error)
	ByConnector(ctx context.Context, connectorID string, from, to time.Time, limit int) ([]sdk.Record, error)
	CountByConnector(ctx context.Context) (map[string]int64, error)
	Timeseries(ctx context.Context, connectorID string, from, to time.Time, bucket time.Duration) ([]TimeseriesBucket, error)
}

// CheckpointStore persists small resumption strings for pull connectors.
type CheckpointStore interface {
	SaveCheckpoint(ctx context.Context, instanceID, key, value string) error
	LoadCheckpoint(ctx context.Context, instanceID, key string) (string, error)
}

// AlertStore manages triggered alerts.
type AlertStore interface {
	InsertAlert(ctx context.Context, a Alert) error
	ListAlerts(ctx context.Context, limit int) ([]Alert, error)
	AckAlert(ctx context.Context, id string, at time.Time) error
}

// RuleStore manages alert rule definitions.
type RuleStore interface {
	SaveRule(ctx context.Context, r AlertRule) error
	DeleteRule(ctx context.Context, id string) error
	ListRules(ctx context.Context) ([]AlertRule, error)
}

// Closer matches io.Closer but kept here to avoid importing io across the
// public storage surface.
type Closer interface {
	Close() error
}

// Backend is the umbrella interface every storage implementation must satisfy.
// It is identical to the v0.1 Storage interface in shape so existing callers
// continue to work; we keep both names so future code can choose the more
// precise term in context.
type Backend interface {
	RecordStore
	CheckpointStore
	AlertStore
	RuleStore
	Closer
}

// Constructor opens a backend given a DSN. The DSN is parsed by the registry
// using url.Parse; constructors may use the entire URL (host, path, query) to
// configure themselves.
//
// Example DSNs:
//
//	duckdb:///var/lib/sunny/sunny.duckdb
//	duckdb://:memory:
//	iceberg://catalog.example.com/warehouse?region=us-east-1   (Phase 1)
//	clickhouse://default:@localhost:9000/sunny                  (Phase 2.5)
type Constructor func(ctx context.Context, dsn *url.URL) (Backend, error)

var (
	registryMu  sync.RWMutex
	constructors = map[string]Constructor{}
)

// Register adds a backend constructor under the given scheme. Calling
// Register twice with the same scheme panics (init-time misconfiguration is
// always a bug).
func Register(scheme string, c Constructor) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := constructors[scheme]; exists {
		panic(fmt.Sprintf("storage: scheme %q already registered", scheme))
	}
	constructors[scheme] = c
}

// Schemes returns the registered scheme names, sorted. Useful for `doctor`
// commands and error messages.
func Schemes() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]string, 0, len(constructors))
	for s := range constructors {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// OpenDSN parses dsn and dispatches to the registered constructor.
//
// A bare filesystem path (no scheme) is treated as duckdb:// for backwards
// compatibility with the v0.1 SUNNY_DATA_DIR + filepath.Join("sunny.duckdb")
// layout. Pass ":memory:" for an in-memory DuckDB.
func OpenDSN(ctx context.Context, dsn string) (Backend, error) {
	if dsn == "" {
		return nil, fmt.Errorf("storage: empty DSN")
	}
	u, err := parseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("storage: parse %q: %w", dsn, err)
	}
	registryMu.RLock()
	c, ok := constructors[u.Scheme]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("storage: unknown scheme %q (registered: %v)", u.Scheme, Schemes())
	}
	return c(ctx, u)
}

// parseDSN handles the two common shapes:
//
//   - "scheme://..." (standard URL)
//   - bare path or ":memory:" → wrapped as duckdb:///<path>
//
// Returns a *url.URL the constructor can read directly.
func parseDSN(dsn string) (*url.URL, error) {
	if dsn == ":memory:" {
		return &url.URL{Scheme: "duckdb", Host: ":memory:"}, nil
	}
	u, err := url.Parse(dsn)
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" {
		// Treat as a filesystem path. url.Parse("/var/lib/sunny.duckdb")
		// produces {Path: "/var/lib/sunny.duckdb"} with empty Scheme.
		return &url.URL{Scheme: "duckdb", Path: dsn}, nil
	}
	return u, nil
}

// init registers the built-in duckdb backend.
func init() {
	Register("duckdb", func(_ context.Context, u *url.URL) (Backend, error) {
		// duckdb://:memory:        → in-memory
		// duckdb:///abs/path.duckdb → file at /abs/path.duckdb
		// duckdb://rel/path.duckdb  → file at rel/path.duckdb
		var path string
		switch {
		case u.Host == ":memory:":
			path = ":memory:"
		case u.Path != "":
			path = u.Path
		default:
			path = u.Host
		}
		return Open(path)
	})
}
