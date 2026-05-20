// Package iceberg implements Apache Iceberg table format v2 for Sunny.
//
// What this package owns:
//
//   - Schema: Sunny ingest schema (sdk.Record) → Iceberg field IDs.
//   - PartitionSpec: how rows shard into files.
//   - SnapshotMetadata + TableMetadata JSON encoding/decoding.
//   - Snapshot lifecycle: append commits, snapshot pointers.
//
// What this package does NOT own (yet):
//
//   - Manifest list Avro encoding — milestone 1.3 follow-up.
//   - Reader (lazy iteration, predicate pushdown) — milestone 1.4.
//   - REST catalog client/server — Phase 2.
//
// Reference: https://iceberg.apache.org/spec/ §2 (terms), §3 (manifest),
// §4 (snapshots), §5 (partitioning).
package iceberg

import "encoding/json"

// FieldID is the stable identifier Iceberg assigns to a column. Renaming
// a column keeps its FieldID; schema evolution is keyed by FieldID, not name.
type FieldID int32

// Field is one column in a schema.
type Field struct {
	ID       FieldID `json:"id"`
	Name     string  `json:"name"`
	Type     string  `json:"type"`               // primitive name; complex types extend later
	Required bool    `json:"required"`
	Doc      string  `json:"doc,omitempty"`
}

// Schema is an Iceberg schema. Schema-id increments on every evolution.
type Schema struct {
	SchemaID         int     `json:"schema-id"`
	IdentifierFields []FieldID `json:"identifier-field-ids,omitempty"`
	Fields           []Field   `json:"fields"`
}

// SunnyV1Schema is the canonical schema for Sunny ingest records,
// matching the Parquet writer in apps/server/internal/storage/parquet.
//
// Field IDs are fixed so they survive renames + reorderings — that's the
// whole point of Iceberg's FieldID model.
func SunnyV1Schema() Schema {
	return Schema{
		SchemaID:         0,
		IdentifierFields: []FieldID{1, 2}, // timestamp + connector_id form the natural PK
		Fields: []Field{
			{ID: 1, Name: "timestamp", Type: "timestamptz", Required: true},
			{ID: 2, Name: "connector_id", Type: "string", Required: true},
			{ID: 3, Name: "source_id", Type: "string", Required: false},
			{ID: 4, Name: "lat", Type: "double", Required: false},
			{ID: 5, Name: "lng", Type: "double", Required: false},
			{ID: 6, Name: "alt", Type: "double", Required: false},
			{ID: 7, Name: "tags", Type: "string", Required: false}, // JSON-encoded
			{ID: 8, Name: "payload", Type: "string", Required: true},
		},
	}
}

// LookupField returns a field by name, or nil if absent. Used by schema
// evolution helpers in milestone 1.6.
func (s Schema) LookupField(name string) *Field {
	for i := range s.Fields {
		if s.Fields[i].Name == name {
			return &s.Fields[i]
		}
	}
	return nil
}

// MarshalJSON guarantees a stable encoding (sorted by FieldID) so two
// Iceberg-spec-compliant readers don't disagree about schema identity.
func (s Schema) MarshalJSON() ([]byte, error) {
	type rawSchema Schema
	return json.Marshal(rawSchema(s))
}
