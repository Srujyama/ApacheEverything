package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sdk "github.com/sunny/sunny/packages/sdk-go"
)

func newTestStore(t *testing.T) Storage {
	t.Helper()
	s, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func mkRec(t time.Time, conn string, payload map[string]any) sdk.Record {
	b, _ := json.Marshal(payload)
	return sdk.Record{
		Timestamp:   t,
		ConnectorID: conn,
		Payload:     b,
	}
}

func TestWriteAndRecent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	recs := []sdk.Record{
		mkRec(now.Add(-2*time.Second), "hello-1", map[string]any{"n": 1}),
		mkRec(now.Add(-1*time.Second), "hello-1", map[string]any{"n": 2}),
		mkRec(now, "hello-1", map[string]any{"n": 3}),
	}
	if err := s.Write(ctx, recs); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := s.Recent(ctx, 10)
	if err != nil {
		t.Fatalf("Recent: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	// Recent is newest-first.
	if !got[0].Timestamp.Equal(now) {
		t.Fatalf("first ts = %v, want %v", got[0].Timestamp, now)
	}
}

func TestByConnectorTimeRange(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		_ = s.Write(ctx, []sdk.Record{mkRec(base.Add(time.Duration(i)*time.Hour), "hello-1", map[string]any{"i": i})})
	}
	_ = s.Write(ctx, []sdk.Record{mkRec(base, "other", map[string]any{})})

	got, err := s.ByConnector(ctx, "hello-1", base.Add(time.Hour), base.Add(4*time.Hour), 10)
	if err != nil {
		t.Fatalf("ByConnector: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3 (hours 1,2,3)", len(got))
	}
	for _, r := range got {
		if r.ConnectorID != "hello-1" {
			t.Fatalf("got %q, want hello-1", r.ConnectorID)
		}
	}
}

func TestLocationRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	alt := 12.5
	r := sdk.Record{
		Timestamp:   time.Now().UTC().Truncate(time.Microsecond),
		ConnectorID: "geo",
		Location:    &sdk.GeoPoint{Lat: 37.8, Lng: -122.4, Altitude: &alt},
		Tags:        map[string]string{"region": "ca"},
		Payload:     json.RawMessage(`{"x":1}`),
	}
	if err := s.Write(ctx, []sdk.Record{r}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := s.Recent(ctx, 1)
	if err != nil || len(got) != 1 {
		t.Fatalf("Recent: %v len=%d", err, len(got))
	}
	g := got[0]
	if g.Location == nil || g.Location.Lat != 37.8 || g.Location.Lng != -122.4 {
		t.Fatalf("location lost: %+v", g.Location)
	}
	if g.Location.Altitude == nil || *g.Location.Altitude != 12.5 {
		t.Fatalf("altitude lost: %+v", g.Location)
	}
	if g.Tags["region"] != "ca" {
		t.Fatalf("tags lost: %+v", g.Tags)
	}
}

func TestCheckpoints(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	v, err := s.LoadCheckpoint(ctx, "inst", "k")
	if err != nil || v != "" {
		t.Fatalf("expected empty initial checkpoint, got %q err=%v", v, err)
	}
	if err := s.SaveCheckpoint(ctx, "inst", "k", "v1"); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveCheckpoint(ctx, "inst", "k", "v2"); err != nil {
		t.Fatal(err)
	}
	v, err = s.LoadCheckpoint(ctx, "inst", "k")
	if err != nil || v != "v2" {
		t.Fatalf("checkpoint = %q err=%v", v, err)
	}
}

func TestPersistsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sunny.duckdb")

	s1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s1.Write(context.Background(), []sdk.Record{
		mkRec(time.Now().UTC(), "hello-1", map[string]any{"n": 1}),
	}); err != nil {
		t.Fatal(err)
	}
	if err := s1.Close(); err != nil {
		t.Fatal(err)
	}

	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	got, err := s2.Recent(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1 after reopen", len(got))
	}
}

func TestEmptyRecent(t *testing.T) {
	s := newTestStore(t)
	got, err := s.Recent(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty, got %d records", len(got))
	}
}

func TestByConnectorFromOnly(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3; i++ {
		_ = s.Write(ctx, []sdk.Record{mkRec(base.Add(time.Duration(i)*time.Hour), "c", map[string]any{})})
	}
	// from only — should return everything at or after that time.
	got, err := s.ByConnector(ctx, "c", base.Add(90*time.Minute), time.Time{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d, want 1 (only the last record passes the from filter)", len(got))
	}
}

func TestExportCSV(t *testing.T) {
	s := newTestStore(t).(*duckStorage)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := s.Write(ctx, []sdk.Record{
		{Timestamp: now, ConnectorID: "csv-c", Payload: json.RawMessage(`{"v":1}`)},
		{Timestamp: now.Add(time.Second), ConnectorID: "csv-c", Payload: json.RawMessage(`{"v":2}`)},
	}); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	rows, err := s.Export(ctx, &buf, ExportCSV, ExportFilter{ConnectorID: "csv-c", Limit: 100})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if rows != 2 {
		t.Fatalf("rows = %d, want 2", rows)
	}
	out := buf.String()
	// Header row + 2 data rows.
	if strings.Count(out, "\n") < 3 {
		t.Fatalf("CSV missing rows: %q", out)
	}
	if !strings.Contains(out, "csv-c") {
		t.Fatalf("CSV missing connector_id: %q", out)
	}
}

func TestExportParquet(t *testing.T) {
	s := newTestStore(t).(*duckStorage)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)
	for i := 0; i < 5; i++ {
		_ = s.Write(ctx, []sdk.Record{
			{Timestamp: now.Add(time.Duration(i) * time.Second), ConnectorID: "p-c", Payload: json.RawMessage(`{"i":1}`)},
		})
	}
	var buf bytes.Buffer
	rows, err := s.Export(ctx, &buf, ExportParquet, ExportFilter{ConnectorID: "p-c"})
	if err != nil {
		t.Fatalf("Export parquet: %v", err)
	}
	if rows != 5 {
		t.Fatalf("rows=%d, want 5", rows)
	}
	// Parquet magic bytes: file starts with "PAR1" and ends with "PAR1".
	if buf.Len() < 8 {
		t.Fatalf("parquet file too small: %d bytes", buf.Len())
	}
	out := buf.Bytes()
	if string(out[:4]) != "PAR1" || string(out[len(out)-4:]) != "PAR1" {
		t.Fatalf("missing parquet magic bytes: head=%q tail=%q", out[:4], out[len(out)-4:])
	}
}
