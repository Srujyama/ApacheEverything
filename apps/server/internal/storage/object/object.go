// Package object defines the ObjectStore contract Sunny uses to talk to
// any blob-storage backend (S3, MinIO, GCS, Azure Blob, local filesystem).
//
// This is the foundation Phase 1 plugs Iceberg + Parquet on top of: every
// data file Sunny writes goes through an ObjectStore, and the Iceberg
// reader fetches manifests + data files through the same interface.
//
// Design constraints:
//
//   - Interface has zero impl-specific options. Each backend exposes a
//     constructor (e.g. NewS3FromEnv, NewLocalFS) and returns an
//     ObjectStore; the type system never sees backend-specific knobs.
//   - All methods take ctx and honor cancellation.
//   - List returns an iterator, not a slice — prefixes can hold millions
//     of keys, and we never want to materialize them all.
//   - Stat is a separate method (not derived from Get) because Iceberg
//     commits rely on cheap existence/size checks during conflict
//     resolution.
//
// Conformance: object_conformance_test.go exercises every method against
// any ObjectStore. Backend impls (s3.go, gcs.go, etc.) only need to
// implement constructors plus a TestXxxConformance(t, impl) wrapper.
package object

import (
	"context"
	"errors"
	"io"
	"iter"
	"time"
)

// ObjectStore is the blob-storage contract. Implementations MUST be safe
// for concurrent use.
type ObjectStore interface {
	// Get opens a stream of the object's bytes. Caller closes.
	// Returns ErrNotFound if no such key exists.
	Get(ctx context.Context, key string) (io.ReadCloser, error)

	// Put uploads body to key. Existing keys are overwritten atomically
	// from the consumer's perspective (no partial-write visibility).
	Put(ctx context.Context, key string, body io.Reader) error

	// Delete removes the key. Returns nil if the key did not exist
	// (idempotent — matches S3 / GCS semantics).
	Delete(ctx context.Context, key string) error

	// List yields every object under prefix. Pagination is the impl's
	// problem; consumers see one continuous iterator.
	List(ctx context.Context, prefix string) iter.Seq2[ObjectInfo, error]

	// Stat returns size + mtime without fetching the body. Returns
	// ErrNotFound if the key doesn't exist.
	Stat(ctx context.Context, key string) (ObjectInfo, error)

	// Scheme is a short identifier ("s3", "gcs", "azure", "file") used in
	// log lines and metrics so operators can tell backends apart.
	Scheme() string
}

// ObjectInfo describes one object.
type ObjectInfo struct {
	Key     string
	Size    int64
	ModTime time.Time
}

// ErrNotFound is returned by Get and Stat when the key doesn't exist.
// Backends MUST wrap their native errors with this so callers can branch
// with errors.Is(err, ErrNotFound).
var ErrNotFound = errors.New("object: key not found")
