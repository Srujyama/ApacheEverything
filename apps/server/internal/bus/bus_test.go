package bus

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	sdk "github.com/sunny/sunny/packages/sdk-go"
)

func mkRecord(id string) sdk.Record {
	return sdk.Record{
		Timestamp:   time.Now(),
		ConnectorID: id,
		Payload:     json.RawMessage(`{}`),
	}
}

func TestBusFanout(t *testing.T) {
	b := New(10, 8)
	a := b.Subscribe(nil, false)
	c := b.Subscribe(nil, false)
	defer a.Close()
	defer c.Close()

	b.Publish(context.Background(), mkRecord("hello"))
	b.Publish(context.Background(), mkRecord("hello"))

	for i, sub := range []*Subscription{a, c} {
		for n := 0; n < 2; n++ {
			select {
			case <-sub.C():
			case <-time.After(time.Second):
				t.Fatalf("sub %d missed record %d", i, n)
			}
		}
	}
}

func TestBusFilter(t *testing.T) {
	b := New(0, 8)
	only := b.Subscribe(func(r sdk.Record) bool { return r.ConnectorID == "keep" }, false)
	defer only.Close()

	b.Publish(context.Background(), mkRecord("drop"))
	b.Publish(context.Background(), mkRecord("keep"))

	select {
	case r := <-only.C():
		if r.ConnectorID != "keep" {
			t.Fatalf("got %q, want keep", r.ConnectorID)
		}
	case <-time.After(time.Second):
		t.Fatal("filter blocked the record we wanted")
	}

	select {
	case r := <-only.C():
		t.Fatalf("filter let through unwanted record %q", r.ConnectorID)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestBusRingReplay(t *testing.T) {
	b := New(3, 8)
	for _, id := range []string{"a", "b", "c", "d"} {
		b.Publish(context.Background(), mkRecord(id))
	}
	sub := b.Subscribe(nil, true)
	defer sub.Close()

	got := make([]string, 0, 3)
	for i := 0; i < 3; i++ {
		select {
		case r := <-sub.C():
			got = append(got, r.ConnectorID)
		case <-time.After(time.Second):
			t.Fatal("missing replay record")
		}
	}
	want := []string{"b", "c", "d"} // ring trimmed "a"
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("replay = %v, want %v", got, want)
		}
	}
}

func TestBusDropsSlowSubscriber(t *testing.T) {
	b := New(0, 2)
	slow := b.Subscribe(nil, false)
	defer slow.Close()

	for i := 0; i < 10; i++ {
		b.Publish(context.Background(), mkRecord("flood"))
	}
	if slow.Dropped() == 0 {
		t.Fatal("expected drops on slow subscriber")
	}
}

func TestBusRecentSnapshot(t *testing.T) {
	b := New(3, 8)
	if got := b.Recent(); len(got) != 0 {
		t.Fatalf("empty bus Recent should be empty, got %d", len(got))
	}
	for _, id := range []string{"a", "b"} {
		b.Publish(context.Background(), mkRecord(id))
	}
	got := b.Recent()
	if len(got) != 2 {
		t.Fatalf("Recent len=%d", len(got))
	}
	// Ring oldest-first.
	if got[0].ConnectorID != "a" || got[1].ConnectorID != "b" {
		t.Fatalf("Recent order: %+v", got)
	}
}

func TestBusZeroRingDoesNotPanic(t *testing.T) {
	b := New(0, 8)
	b.Publish(context.Background(), mkRecord("once"))
	if got := b.Recent(); len(got) != 0 {
		t.Fatalf("ringSize=0 should retain nothing, got %d", len(got))
	}
}

func TestBusUnsubscribedDoesNotReceive(t *testing.T) {
	b := New(0, 8)
	sub := b.Subscribe(nil, false)
	sub.Close()
	// After Close, publishing must not panic and must not deliver.
	b.Publish(context.Background(), mkRecord("orphan"))
	select {
	case _, ok := <-sub.C():
		if ok {
			t.Fatal("got record on closed subscription")
		}
	default:
	}
}
