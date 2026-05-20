// Package parquet writes Sunny ingest records as Apache Parquet files.
//
// Each file is a self-describing columnar bundle that Spark, Trino, DuckDB,
// PyArrow, and PyIceberg can all read directly. The schema mirrors the
// DuckDB events table in apps/server/internal/storage/storage.go:
//
//   timestamp     TIMESTAMP(microsecond, UTC)
//   connector_id  STRING
//   source_id     STRING       nullable
//   lat           DOUBLE       nullable
//   lng           DOUBLE       nullable
//   alt           DOUBLE       nullable
//   tags          STRING (JSON) nullable
//   payload       STRING (JSON)
//
// Phase 1.5 of the lakehouse plan. Subsequent milestones (1.3, 1.4)
// wrap this writer with Iceberg manifest emission.
package parquet

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/apache/arrow-go/v18/arrow/memory"
	"github.com/apache/arrow-go/v18/parquet"
	"github.com/apache/arrow-go/v18/parquet/compress"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"

	sdk "github.com/sunny/sunny/packages/sdk-go"
)

// SunnySchema is the Arrow schema for Sunny ingest records. Exported for
// reuse by the Iceberg writer and by tests that need to construct compatible
// record batches.
func SunnySchema() *arrow.Schema {
	return arrow.NewSchema([]arrow.Field{
		{Name: "timestamp", Type: arrow.FixedWidthTypes.Timestamp_us, Nullable: false},
		{Name: "connector_id", Type: arrow.BinaryTypes.String, Nullable: false},
		{Name: "source_id", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "lat", Type: arrow.PrimitiveTypes.Float64, Nullable: true},
		{Name: "lng", Type: arrow.PrimitiveTypes.Float64, Nullable: true},
		{Name: "alt", Type: arrow.PrimitiveTypes.Float64, Nullable: true},
		{Name: "tags", Type: arrow.BinaryTypes.String, Nullable: true},
		{Name: "payload", Type: arrow.BinaryTypes.String, Nullable: false},
	}, nil)
}

// WriterOptions tune the output file. Zero-values give defaults sized for
// observability workloads (rows up to a few KB each, mixed columns).
type WriterOptions struct {
	// RowGroupSize targets bytes per row group. 128 MiB by default — large
	// enough that scan parallelism dominates seek cost on object storage.
	RowGroupSize int64

	// Compression. CompressionZstd by default (best ratio for our mostly-
	// text payloads). Use Snappy for lower CPU on hot ingest paths.
	Compression compress.Compression

	// DictionaryEnabled. On by default; cuts size dramatically when
	// connector_id and source_id have low cardinality.
	DictionaryEnabled bool
}

// DefaultOptions returns sensible defaults.
func DefaultOptions() WriterOptions {
	return WriterOptions{
		RowGroupSize:      128 << 20,
		Compression:       compress.Codecs.Zstd,
		DictionaryEnabled: true,
	}
}

// Writer writes ingest records to a single Parquet file. It accumulates
// records in an in-memory builder until WriteBatch is called (which flushes
// one row group). Close finalizes the file footer.
//
// Safe for use from a single goroutine. Concurrent Append from multiple
// goroutines is unsupported by design — the call site owns batching.
type Writer struct {
	out         io.Writer
	closer      io.Closer
	pq          *pqarrow.FileWriter
	bldr        *array.RecordBuilder
	pool        memory.Allocator
	pending     int // rows in the current Arrow RecordBuilder
	rowGroupCap int
}

// NewWriter builds a Writer that streams to dst. dst is NOT closed by the
// Writer; the caller owns that file/stream. Pass nil opts for defaults.
func NewWriter(dst io.Writer, opts *WriterOptions) (*Writer, error) {
	if opts == nil {
		o := DefaultOptions()
		opts = &o
	}
	pool := memory.NewGoAllocator()
	schema := SunnySchema()

	props := parquet.NewWriterProperties(
		parquet.WithCompression(opts.Compression),
		parquet.WithDictionaryDefault(opts.DictionaryEnabled),
	)
	arrowProps := pqarrow.NewArrowWriterProperties(
		pqarrow.WithStoreSchema(),
	)
	pq, err := pqarrow.NewFileWriter(schema, dst, props, arrowProps)
	if err != nil {
		return nil, fmt.Errorf("parquet: new writer: %w", err)
	}
	return &Writer{
		out:         dst,
		pq:          pq,
		bldr:        array.NewRecordBuilder(pool, schema),
		pool:        pool,
		rowGroupCap: 64 << 10, // ~64k rows per row group target
	}, nil
}

// Append buffers one record. Call Flush to emit a row group, or rely on
// Close to flush whatever's left.
func (w *Writer) Append(r sdk.Record) error {
	if w.bldr == nil {
		return fmt.Errorf("parquet: writer closed")
	}
	// timestamp
	tsCol := w.bldr.Field(0).(*array.TimestampBuilder)
	tsCol.Append(arrow.Timestamp(r.Timestamp.UnixMicro()))
	// connector_id
	connCol := w.bldr.Field(1).(*array.StringBuilder)
	connCol.Append(r.ConnectorID)
	// source_id
	srcCol := w.bldr.Field(2).(*array.StringBuilder)
	if r.SourceID == "" {
		srcCol.AppendNull()
	} else {
		srcCol.Append(r.SourceID)
	}
	// lat / lng / alt
	latCol := w.bldr.Field(3).(*array.Float64Builder)
	lngCol := w.bldr.Field(4).(*array.Float64Builder)
	altCol := w.bldr.Field(5).(*array.Float64Builder)
	if r.Location == nil {
		latCol.AppendNull()
		lngCol.AppendNull()
		altCol.AppendNull()
	} else {
		latCol.Append(r.Location.Lat)
		lngCol.Append(r.Location.Lng)
		if r.Location.Altitude == nil {
			altCol.AppendNull()
		} else {
			altCol.Append(*r.Location.Altitude)
		}
	}
	// tags
	tagsCol := w.bldr.Field(6).(*array.StringBuilder)
	if len(r.Tags) == 0 {
		tagsCol.AppendNull()
	} else {
		b, err := json.Marshal(r.Tags)
		if err != nil {
			return fmt.Errorf("parquet: marshal tags: %w", err)
		}
		tagsCol.Append(string(b))
	}
	// payload
	payloadCol := w.bldr.Field(7).(*array.StringBuilder)
	if len(r.Payload) == 0 {
		payloadCol.Append("{}")
	} else {
		payloadCol.Append(string(r.Payload))
	}

	w.pending++
	if w.pending >= w.rowGroupCap {
		return w.Flush()
	}
	return nil
}

// AppendAll is a convenience for batch ingest.
func (w *Writer) AppendAll(ctx context.Context, records []sdk.Record) error {
	for i, r := range records {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := w.Append(r); err != nil {
			return fmt.Errorf("parquet: append %d: %w", i, err)
		}
	}
	return nil
}

// Flush emits the current builder as one row group. Safe to call when
// pending == 0 (no-op).
func (w *Writer) Flush() error {
	if w.pending == 0 {
		return nil
	}
	rec := w.bldr.NewRecord()
	defer rec.Release()
	if err := w.pq.Write(rec); err != nil {
		return fmt.Errorf("parquet: write row group: %w", err)
	}
	w.pending = 0
	return nil
}

// Close flushes any pending rows, finalizes the file footer, and releases
// resources. Does NOT close the underlying writer (caller owns it).
func (w *Writer) Close() error {
	if w.bldr == nil {
		return nil
	}
	defer func() {
		w.bldr.Release()
		w.bldr = nil
	}()
	if err := w.Flush(); err != nil {
		return err
	}
	return w.pq.Close()
}

// Rows returns the number of rows in the current (un-flushed) buffer.
// Exposed for metrics + tests.
func (w *Writer) Rows() int { return w.pending }
