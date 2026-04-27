package storage

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	sdk "github.com/sunny/sunny/packages/sdk-go"
)

func TestTimeseriesBucketing(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 4, 26, 0, 0, 0, 0, time.UTC)

	for i := 0; i < 5; i++ {
		_ = s.Write(ctx, []sdk.Record{
			{Timestamp: base.Add(time.Duration(i) * 10 * time.Second), ConnectorID: "a", Payload: json.RawMessage(`{}`)},
		})
	}
	for i := 0; i < 3; i++ {
		_ = s.Write(ctx, []sdk.Record{
			{Timestamp: base.Add(2*time.Minute + time.Duration(i)*15*time.Second), ConnectorID: "a", Payload: json.RawMessage(`{}`)},
		})
	}
	_ = s.Write(ctx, []sdk.Record{
		{Timestamp: base, ConnectorID: "b", Payload: json.RawMessage(`{}`)},
	})

	// 1-minute buckets across whole table for connector a.
	got, err := s.Timeseries(ctx, "a", time.Time{}, time.Time{}, time.Minute)
	if err != nil {
		t.Fatalf("Timeseries: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("buckets = %d, want 2 (minute 0 and minute 2)", len(got))
	}
	if got[0].Count != 5 {
		t.Fatalf("first bucket = %d, want 5", got[0].Count)
	}
	if got[1].Count != 3 {
		t.Fatalf("second bucket = %d, want 3", got[1].Count)
	}

	// All connectors, same range.
	all, err := s.Timeseries(ctx, "", time.Time{}, time.Time{}, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	var sum int64
	for _, b := range all {
		sum += b.Count
	}
	if sum != 9 {
		t.Fatalf("sum across all = %d, want 9", sum)
	}
}

func TestCountByConnector(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()
	_ = s.Write(ctx, []sdk.Record{
		{Timestamp: now, ConnectorID: "a", Payload: json.RawMessage(`{}`)},
		{Timestamp: now, ConnectorID: "a", Payload: json.RawMessage(`{}`)},
		{Timestamp: now, ConnectorID: "b", Payload: json.RawMessage(`{}`)},
	})
	got, err := s.CountByConnector(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got["a"] != 2 || got["b"] != 1 {
		t.Fatalf("counts = %v", got)
	}
}

func TestAlertRulesAndAlerts(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	r := AlertRule{
		ID:         "r1",
		Name:       "Critical events",
		Enabled:    true,
		SeverityIn: []string{"emergency", "critical"},
	}
	if err := s.SaveRule(ctx, r); err != nil {
		t.Fatal(err)
	}
	rules, err := s.ListRules(ctx)
	if err != nil || len(rules) != 1 || len(rules[0].SeverityIn) != 2 {
		t.Fatalf("ListRules: %+v err=%v", rules, err)
	}

	now := time.Now().UTC()
	a := Alert{
		ID: "a1", RuleID: "r1", RuleName: "Critical events",
		ConnectorID: "earthquakes", Severity: "critical",
		Headline: "M5.0 quake near Berkeley", Triggered: now,
		Tags: map[string]string{"severity": "critical"},
	}
	if err := s.InsertAlert(ctx, a); err != nil {
		t.Fatal(err)
	}
	alerts, err := s.ListAlerts(ctx, 10)
	if err != nil || len(alerts) != 1 || alerts[0].Headline != "M5.0 quake near Berkeley" {
		t.Fatalf("ListAlerts: %+v err=%v", alerts, err)
	}
	if alerts[0].Acked != nil {
		t.Fatalf("alert acked unexpectedly")
	}
	if err := s.AckAlert(ctx, "a1", now); err != nil {
		t.Fatal(err)
	}
	alerts, _ = s.ListAlerts(ctx, 10)
	if alerts[0].Acked == nil {
		t.Fatal("alert should be acked")
	}
}
