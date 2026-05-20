// snapshot.go is the on-disk JSON encoding of Iceberg snapshots + table
// metadata (the v2/metadata/vN.metadata.json files).
//
// We keep this faithful to the spec so external readers — Spark, Trino,
// PyIceberg, DuckDB-iceberg-extension — can consume Sunny tables without
// special-casing. Reference: spec §4 (snapshots), §6 (table metadata).

package iceberg

import (
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"time"
)

// SnapshotID is the spec-required 64-bit identifier for a snapshot.
// Iceberg requires snapshot IDs to be unique across a table; we generate
// them from crypto/rand to defeat collisions across concurrent writers.
type SnapshotID int64

// NewSnapshotID returns a random non-negative SnapshotID.
func NewSnapshotID() SnapshotID {
	var b [8]byte
	_, _ = rand.Read(b[:])
	id := int64(binary.BigEndian.Uint64(b[:]) & 0x7FFFFFFFFFFFFFFF)
	return SnapshotID(id)
}

// Operation describes the change a snapshot represents.
type Operation string

const (
	OpAppend    Operation = "append"
	OpReplace   Operation = "replace"
	OpOverwrite Operation = "overwrite"
	OpDelete    Operation = "delete"
)

// Summary is the per-snapshot metric block. Iceberg uses these for both
// debuggability and reader optimization (e.g. "skip this snapshot if its
// total-records is 0").
type Summary map[string]string

// Snapshot describes one committed change to the table.
type Snapshot struct {
	SnapshotID       SnapshotID  `json:"snapshot-id"`
	ParentSnapshotID *SnapshotID `json:"parent-snapshot-id,omitempty"`
	SequenceNumber   int64       `json:"sequence-number"`
	TimestampMs      int64       `json:"timestamp-ms"`
	ManifestList     string      `json:"manifest-list"`
	Summary          Summary     `json:"summary"`
	SchemaID         int         `json:"schema-id,omitempty"`
}

// SnapshotLogEntry is one row of the table's snapshot history. Readers
// follow this log to enumerate snapshots in commit order.
type SnapshotLogEntry struct {
	TimestampMs int64      `json:"timestamp-ms"`
	SnapshotID  SnapshotID `json:"snapshot-id"`
}

// MetadataLogEntry tracks every metadata.json the table has produced.
// External tools use this for retention policies + GC.
type MetadataLogEntry struct {
	TimestampMs  int64  `json:"timestamp-ms"`
	MetadataFile string `json:"metadata-file"`
}

// TableMetadata is the root object stored at
// <table>/metadata/<n>-<uuid>.metadata.json. Every commit produces a new
// one; the catalog (Phase 2) tracks the current pointer.
type TableMetadata struct {
	FormatVersion   int                `json:"format-version"`
	TableUUID       string             `json:"table-uuid"`
	Location        string             `json:"location"`
	LastSequenceNum int64              `json:"last-sequence-number"`
	LastUpdatedMs   int64              `json:"last-updated-ms"`
	LastColumnID    int                `json:"last-column-id"`
	Schemas         []Schema           `json:"schemas"`
	CurrentSchemaID int                `json:"current-schema-id"`
	PartitionSpecs  []PartitionSpec    `json:"partition-specs"`
	DefaultSpecID   int                `json:"default-spec-id"`
	LastPartitionID int                `json:"last-partition-id"`
	Properties      map[string]string  `json:"properties,omitempty"`
	CurrentSnapshotID *SnapshotID      `json:"current-snapshot-id,omitempty"`
	Snapshots       []Snapshot         `json:"snapshots"`
	SnapshotLog     []SnapshotLogEntry `json:"snapshot-log"`
	MetadataLog     []MetadataLogEntry `json:"metadata-log"`
}

// NewTableMetadata builds an empty v2 metadata document for a brand-new
// table. Subsequent commits append snapshots via Commit().
func NewTableMetadata(uuid, location string) TableMetadata {
	schema := SunnyV1Schema()
	spec := DefaultPartitionSpec()
	now := time.Now().UTC().UnixMilli()
	return TableMetadata{
		FormatVersion:    2,
		TableUUID:        uuid,
		Location:         location,
		LastSequenceNum:  0,
		LastUpdatedMs:    now,
		LastColumnID:     int(schema.Fields[len(schema.Fields)-1].ID),
		Schemas:          []Schema{schema},
		CurrentSchemaID:  schema.SchemaID,
		PartitionSpecs:   []PartitionSpec{spec},
		DefaultSpecID:    spec.SpecID,
		LastPartitionID:  int(spec.Fields[len(spec.Fields)-1].FieldID),
		Properties:       map[string]string{"sunny.created-by": "sunny.iceberg.v1"},
		Snapshots:        []Snapshot{},
		SnapshotLog:      []SnapshotLogEntry{},
		MetadataLog:      []MetadataLogEntry{},
	}
}

// Commit appends a snapshot. Returns the new metadata; the original is
// unchanged (immutable update pattern — easier reasoning under
// concurrency since callers can take the result-or-keep-old based on a
// CAS).
func (m TableMetadata) Commit(snap Snapshot) TableMetadata {
	now := time.Now().UTC().UnixMilli()
	if snap.TimestampMs == 0 {
		snap.TimestampMs = now
	}
	snap.SequenceNumber = m.LastSequenceNum + 1

	out := m
	out.LastSequenceNum = snap.SequenceNumber
	out.LastUpdatedMs = now
	out.Snapshots = append(append([]Snapshot{}, m.Snapshots...), snap)
	id := snap.SnapshotID
	out.CurrentSnapshotID = &id
	out.SnapshotLog = append(append([]SnapshotLogEntry{}, m.SnapshotLog...), SnapshotLogEntry{
		TimestampMs: snap.TimestampMs,
		SnapshotID:  snap.SnapshotID,
	})
	return out
}

// Encode marshals the metadata to canonical JSON.
func (m TableMetadata) Encode() ([]byte, error) {
	return json.MarshalIndent(m, "", "  ")
}

// DecodeTableMetadata parses a metadata.json blob.
func DecodeTableMetadata(b []byte) (TableMetadata, error) {
	var m TableMetadata
	err := json.Unmarshal(b, &m)
	return m, err
}
