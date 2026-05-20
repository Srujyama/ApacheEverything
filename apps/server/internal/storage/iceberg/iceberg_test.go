package iceberg

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSunnyV1Schema_StableFieldIDs(t *testing.T) {
	t.Parallel()
	s := SunnyV1Schema()
	wantIDs := map[string]FieldID{
		"timestamp":    1,
		"connector_id": 2,
		"source_id":    3,
		"lat":          4,
		"lng":          5,
		"alt":          6,
		"tags":         7,
		"payload":      8,
	}
	if len(s.Fields) != len(wantIDs) {
		t.Fatalf("field count = %d, want %d", len(s.Fields), len(wantIDs))
	}
	for _, f := range s.Fields {
		want, ok := wantIDs[f.Name]
		if !ok {
			t.Errorf("unexpected field %q", f.Name)
			continue
		}
		if f.ID != want {
			t.Errorf("field %q: id = %d, want %d (CHANGING THIS IS A WIRE-BREAK)",
				f.Name, f.ID, want)
		}
	}
}

func TestLookupField(t *testing.T) {
	t.Parallel()
	s := SunnyV1Schema()
	if f := s.LookupField("connector_id"); f == nil || f.ID != 2 {
		t.Fatalf("LookupField connector_id = %+v", f)
	}
	if f := s.LookupField("does-not-exist"); f != nil {
		t.Fatalf("expected nil, got %+v", f)
	}
}

func TestDefaultPartitionSpec(t *testing.T) {
	t.Parallel()
	spec := DefaultPartitionSpec()
	if len(spec.Fields) != 2 {
		t.Fatalf("fields = %d", len(spec.Fields))
	}
	if spec.Fields[0].Transform != TransformIdentity || spec.Fields[0].Name != "connector_id" {
		t.Errorf("field 0 = %+v", spec.Fields[0])
	}
	if spec.Fields[1].Transform != TransformDay || spec.Fields[1].Name != "day_ts" {
		t.Errorf("field 1 = %+v", spec.Fields[1])
	}
}

func TestNewSnapshotID_NonNegativeAndUnique(t *testing.T) {
	t.Parallel()
	seen := map[SnapshotID]bool{}
	for i := 0; i < 1000; i++ {
		id := NewSnapshotID()
		if id < 0 {
			t.Fatalf("negative SnapshotID: %d", id)
		}
		if seen[id] {
			t.Fatalf("collision at %d", id)
		}
		seen[id] = true
	}
}

func TestTableMetadata_CommitAppendsSnapshot(t *testing.T) {
	t.Parallel()
	m := NewTableMetadata("uuid-1", "s3://bucket/tbl")
	if m.CurrentSnapshotID != nil {
		t.Fatalf("fresh metadata should have nil current snapshot")
	}
	snap := Snapshot{
		SnapshotID:   NewSnapshotID(),
		ManifestList: "s3://bucket/tbl/metadata/snap-1.avro",
		Summary:      Summary{"total-records": "42", "added-files-size": "1234", "operation": string(OpAppend)},
	}
	m2 := m.Commit(snap)
	if len(m2.Snapshots) != 1 {
		t.Fatalf("snapshots = %d", len(m2.Snapshots))
	}
	if m2.CurrentSnapshotID == nil || *m2.CurrentSnapshotID != snap.SnapshotID {
		t.Fatalf("CurrentSnapshotID = %v", m2.CurrentSnapshotID)
	}
	if len(m.Snapshots) != 0 {
		t.Fatalf("original metadata mutated; commit should be immutable")
	}
	if m2.LastSequenceNum != 1 {
		t.Fatalf("seq = %d", m2.LastSequenceNum)
	}
	// Second commit increments.
	snap2 := snap
	snap2.SnapshotID = NewSnapshotID()
	m3 := m2.Commit(snap2)
	if m3.LastSequenceNum != 2 {
		t.Fatalf("seq after 2nd commit = %d", m3.LastSequenceNum)
	}
	if len(m3.SnapshotLog) != 2 {
		t.Fatalf("snapshot-log len = %d", len(m3.SnapshotLog))
	}
}

func TestTableMetadata_RoundTripJSON(t *testing.T) {
	t.Parallel()
	m := NewTableMetadata("uuid-rt", "file:///tmp/tbl")
	m = m.Commit(Snapshot{
		SnapshotID:   42,
		ManifestList: "manifest.avro",
		Summary:      Summary{"k": "v"},
	})
	b, err := m.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"format-version": 2`) {
		t.Errorf("missing format-version in output:\n%s", b)
	}
	if !strings.Contains(string(b), `"schema-id": 0`) {
		t.Errorf("missing schema-id in output")
	}
	round, err := DecodeTableMetadata(b)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if round.TableUUID != "uuid-rt" || round.LastSequenceNum != 1 || len(round.Snapshots) != 1 {
		t.Fatalf("round-tripped metadata = %+v", round)
	}
}

func TestSchema_MarshalJSON_Stable(t *testing.T) {
	t.Parallel()
	s := SunnyV1Schema()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	// JSON must contain field IDs in source order — Iceberg readers don't
	// require it but external tools like to diff metadata files.
	wantOrder := []string{"timestamp", "connector_id", "source_id", "lat", "lng", "alt", "tags", "payload"}
	prev := -1
	for _, n := range wantOrder {
		i := strings.Index(string(b), `"name":"`+n+`"`)
		if i < 0 {
			t.Fatalf("missing %q in JSON", n)
		}
		if i < prev {
			t.Fatalf("field %q appears before previous (got %d, prev %d)", n, i, prev)
		}
		prev = i
	}
}
