// property_test.go runs randomized writes against the storage layer and
// asserts invariants that must hold no matter what records are pushed in:
//
//  1. Every record we Write is readable via Recent or ByConnector.
//  2. Recent's results are sorted timestamp DESC, no duplicates.
//  3. CountByConnector totals across connectors equal len(written).
//  4. Timeseries bucket counts sum to the per-connector total.
//  5. SaveCheckpoint / LoadCheckpoint round-trip every key.
//
// Phase 0.2 of PLAN.md. Future contributors can swap in
// github.com/leanovate/gopter or similar; right now we use math/rand for
// determinism + zero deps.

package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"testing"
	"time"

	sdk "github.com/sunny/sunny/packages/sdk-go"
)

// genRecord produces a record with random-ish fields.
func genRecord(rng *rand.Rand, i int) sdk.Record {
	connectors := []string{"earthquakes", "weather", "rivers", "fires", "air"}
	severities := []string{"info", "warning", "critical", "emergency"}
	c := connectors[rng.Intn(len(connectors))]
	s := severities[rng.Intn(len(severities))]
	hasLoc := rng.Float64() < 0.7
	var loc *sdk.GeoPoint
	if hasLoc {
		loc = &sdk.GeoPoint{
			Lat: -90 + rng.Float64()*180,
			Lng: -180 + rng.Float64()*360,
		}
	}
	return sdk.Record{
		Timestamp:   time.Now().UTC().Add(-time.Duration(rng.Intn(3600)) * time.Second),
		ConnectorID: c,
		SourceID:    fmt.Sprintf("src-%d", i),
		Location:    loc,
		Tags:        map[string]string{"severity": s},
		Payload:     json.RawMessage(fmt.Sprintf(`{"i":%d}`, i)),
	}
}

func TestProperty_WrittenRecordsAreRecoverable(t *testing.T) {
	t.Parallel()
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	rng := rand.New(rand.NewSource(42))
	const n = 500
	records := make([]sdk.Record, n)
	for i := range records {
		records[i] = genRecord(rng, i)
	}
	// Write in small random batches to exercise the batching path.
	ctx := context.Background()
	start := 0
	for start < n {
		batch := 1 + rng.Intn(25)
		if start+batch > n {
			batch = n - start
		}
		if err := store.Write(ctx, records[start:start+batch]); err != nil {
			t.Fatalf("Write: %v", err)
		}
		start += batch
	}

	// Invariant 1: total count via aggregator equals n.
	counts, err := store.CountByConnector(ctx)
	if err != nil {
		t.Fatal(err)
	}
	total := int64(0)
	for _, c := range counts {
		total += c
	}
	if total != n {
		t.Fatalf("count total = %d, want %d", total, n)
	}

	// Invariant 2: Recent returns n records, sorted DESC by timestamp.
	got, err := store.Recent(ctx, n+10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != n {
		t.Fatalf("Recent returned %d, want %d", len(got), n)
	}
	for i := 1; i < len(got); i++ {
		if got[i].Timestamp.After(got[i-1].Timestamp) {
			t.Fatalf("Recent not sorted DESC at i=%d: %v > %v", i, got[i].Timestamp, got[i-1].Timestamp)
		}
	}

	// Invariant 3: per-connector counts add up.
	perConn := map[string]int64{}
	for _, r := range records {
		perConn[r.ConnectorID]++
	}
	for c, want := range perConn {
		if got := counts[c]; got != want {
			t.Errorf("count[%s] = %d, want %d", c, got, want)
		}
	}

	// Invariant 4: Timeseries bucket totals match per-connector counts.
	now := time.Now().UTC()
	from := now.Add(-2 * time.Hour)
	for c, want := range perConn {
		buckets, err := store.Timeseries(ctx, c, from, now, time.Minute)
		if err != nil {
			t.Fatalf("Timeseries(%s): %v", c, err)
		}
		var sum int64
		for _, b := range buckets {
			sum += b.Count
		}
		if sum != want {
			t.Errorf("timeseries[%s] sum = %d, want %d", c, sum, want)
		}
	}
}

func TestProperty_CheckpointRoundTrips(t *testing.T) {
	t.Parallel()
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()
	rng := rand.New(rand.NewSource(43))

	type ik struct{ inst, key string }
	cases := make([]struct {
		ik
		val string
	}, 50)
	for i := range cases {
		cases[i].ik = ik{
			inst: fmt.Sprintf("inst-%d", rng.Intn(10)),
			key:  fmt.Sprintf("key-%d", rng.Intn(20)),
		}
		cases[i].val = fmt.Sprintf("v-%d-%d", i, rng.Int())
	}
	// Last-write-wins by (inst, key); collapse to expected map.
	expected := map[ik]string{}
	for _, c := range cases {
		if err := store.SaveCheckpoint(ctx, c.inst, c.key, c.val); err != nil {
			t.Fatalf("SaveCheckpoint: %v", err)
		}
		expected[c.ik] = c.val
	}
	for k, want := range expected {
		got, err := store.LoadCheckpoint(ctx, k.inst, k.key)
		if err != nil {
			t.Fatalf("LoadCheckpoint: %v", err)
		}
		if got != want {
			t.Errorf("LoadCheckpoint(%s,%s) = %q, want %q", k.inst, k.key, got, want)
		}
	}
}

func TestProperty_ByConnectorRespectsTimeWindow(t *testing.T) {
	t.Parallel()
	store, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()

	base := time.Now().UTC().Truncate(time.Second)
	var records []sdk.Record
	for i := 0; i < 200; i++ {
		records = append(records, sdk.Record{
			Timestamp:   base.Add(time.Duration(i) * time.Second),
			ConnectorID: "win",
			SourceID:    fmt.Sprintf("s%d", i),
			Payload:     json.RawMessage(`{}`),
		})
	}
	if err := store.Write(ctx, records); err != nil {
		t.Fatal(err)
	}

	from := base.Add(50 * time.Second)
	to := base.Add(150 * time.Second)
	got, err := store.ByConnector(ctx, "win", from, to, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 100 {
		t.Fatalf("ByConnector window len = %d, want 100", len(got))
	}
	for _, r := range got {
		if r.Timestamp.Before(from) || !r.Timestamp.Before(to) {
			t.Errorf("record %s outside window: %v", r.SourceID, r.Timestamp)
		}
	}
}
