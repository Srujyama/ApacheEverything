// partition.go encodes the Iceberg "partition spec" — how rows shard into
// files within a single table.
//
// Sunny's default partition spec is `connector_id` plus `day(timestamp)`.
// That gives:
//
//   - Time-range queries prune to one or two day directories.
//   - Per-connector queries skip every other connector's files.
//
// Other transforms (year, month, hour, bucket, truncate) are listed in
// PartitionTransform but aren't all wired up to the writer yet —
// extension point for Phase 1.8 (Z-order) and beyond.

package iceberg

// PartitionTransform is the function applied to a source column to derive
// the partition value. Mirrors §4.2 of the Iceberg spec.
type PartitionTransform string

const (
	TransformIdentity PartitionTransform = "identity"
	TransformYear     PartitionTransform = "year"
	TransformMonth    PartitionTransform = "month"
	TransformDay      PartitionTransform = "day"
	TransformHour     PartitionTransform = "hour"
	TransformBucket   PartitionTransform = "bucket" // bucket[N] — N goes in params
	TransformTruncate PartitionTransform = "truncate" // truncate[N]
)

// PartitionField is one entry in a PartitionSpec.
type PartitionField struct {
	SourceID  FieldID            `json:"source-id"`
	FieldID   FieldID            `json:"field-id"`
	Name      string             `json:"name"`
	Transform PartitionTransform `json:"transform"`
}

// PartitionSpec is the full partitioning rule for a table.
type PartitionSpec struct {
	SpecID int              `json:"spec-id"`
	Fields []PartitionField `json:"fields"`
}

// DefaultPartitionSpec returns the Sunny default: partition by
// connector_id, then bucket data by day(timestamp).
func DefaultPartitionSpec() PartitionSpec {
	return PartitionSpec{
		SpecID: 0,
		Fields: []PartitionField{
			{
				SourceID:  2, // connector_id
				FieldID:   1000,
				Name:      "connector_id",
				Transform: TransformIdentity,
			},
			{
				SourceID:  1, // timestamp
				FieldID:   1001,
				Name:      "day_ts",
				Transform: TransformDay,
			},
		},
	}
}
