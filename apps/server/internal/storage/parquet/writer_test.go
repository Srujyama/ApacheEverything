package parquet

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/parquet/file"
	"github.com/apache/arrow-go/v18/parquet/pqarrow"

	sdk "github.com/sunny/sunny/packages/sdk-go"
)

func TestWriter_RoundTrip(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	w, err := NewWriter(&buf, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 5, 19, 23, 0, 0, 0, time.UTC)
	alt := 42.0
	records := []sdk.Record{
		{
			Timestamp:   now,
			ConnectorID: "earthquakes",
			SourceID:    "q1",
			Location:    &sdk.GeoPoint{Lat: 37.8, Lng: -122.3, Altitude: &alt},
			Tags:        map[string]string{"severity": "critical", "region": "us-west"},
			Payload:     json.RawMessage(`{"magnitude":5.1,"place":"Berkeley"}`),
		},
		{
			Timestamp:   now.Add(time.Second),
			ConnectorID: "earthquakes",
			SourceID:    "", // intentionally blank
			Tags:        nil,
			Payload:     json.RawMessage(`{"magnitude":3.0}`),
		},
		{
			Timestamp:   now.Add(2 * time.Second),
			ConnectorID: "weather",
			SourceID:    "stn-7",
			Location:    &sdk.GeoPoint{Lat: 40.0, Lng: -110.0}, // no altitude
			Payload:     json.RawMessage(`{"temp":72}`),
		},
	}
	if err := w.AppendAll(context.Background(), records); err != nil {
		t.Fatalf("AppendAll: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Sanity: file ends with the Parquet magic ("PAR1").
	data := buf.Bytes()
	if len(data) < 8 {
		t.Fatalf("file too short: %d bytes", len(data))
	}
	if string(data[:4]) != "PAR1" {
		t.Fatalf("missing leading magic: %q", string(data[:4]))
	}
	if string(data[len(data)-4:]) != "PAR1" {
		t.Fatalf("missing trailing magic: %q", string(data[len(data)-4:]))
	}

	// Read back via Arrow.
	pf, err := file.NewParquetReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	defer pf.Close()
	if int(pf.NumRows()) != len(records) {
		t.Fatalf("NumRows = %d, want %d", pf.NumRows(), len(records))
	}

	// Read via pqarrow to verify column types.
	ar, err := pqarrow.NewFileReader(pf, pqarrow.ArrowReadProperties{}, nil)
	if err != nil {
		t.Fatalf("pqarrow reader: %v", err)
	}
	schema, err := ar.Schema()
	if err != nil {
		t.Fatal(err)
	}
	want := SunnySchema()
	if schema.NumFields() != want.NumFields() {
		t.Fatalf("schema field count = %d, want %d", schema.NumFields(), want.NumFields())
	}
	for i := 0; i < want.NumFields(); i++ {
		gotF := schema.Field(i)
		wantF := want.Field(i)
		if gotF.Name != wantF.Name {
			t.Errorf("field[%d] name = %q, want %q", i, gotF.Name, wantF.Name)
		}
		if !arrow.TypeEqual(gotF.Type, wantF.Type) {
			t.Errorf("field[%d] %s type = %v, want %v", i, gotF.Name, gotF.Type, wantF.Type)
		}
	}
}

func TestWriter_FlushesAtRowGroupCap(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	w, err := NewWriter(&buf, nil)
	if err != nil {
		t.Fatal(err)
	}
	w.rowGroupCap = 100 // tiny cap so we exercise multi-row-group writes
	for i := 0; i < 250; i++ {
		err := w.Append(sdk.Record{
			Timestamp:   time.Now(),
			ConnectorID: "test",
			SourceID:    fmt.Sprintf("s%d", i),
			Payload:     json.RawMessage(`{}`),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	pf, _ := file.NewParquetReader(bytes.NewReader(buf.Bytes()))
	defer pf.Close()
	if pf.NumRows() != 250 {
		t.Fatalf("NumRows = %d", pf.NumRows())
	}
	if pf.NumRowGroups() < 3 {
		t.Fatalf("expected ≥3 row groups, got %d", pf.NumRowGroups())
	}
}

func TestWriter_EmptyFileIsValid(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	w, err := NewWriter(&buf, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if string(buf.Bytes()[:4]) != "PAR1" {
		t.Fatalf("empty file lacks magic")
	}
	pf, err := file.NewParquetReader(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("read empty: %v", err)
	}
	defer pf.Close()
	if pf.NumRows() != 0 {
		t.Fatalf("empty file NumRows = %d", pf.NumRows())
	}
}
