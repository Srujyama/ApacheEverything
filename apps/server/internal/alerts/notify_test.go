package alerts

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sunny/sunny/apps/server/internal/storage"
)

// fakeNotifier counts calls and can be programmed to fail N times before
// succeeding.
type fakeNotifier struct {
	name        string
	failures    int32 // remaining failures
	permanent   bool
	calls       atomic.Int32
}

func (f *fakeNotifier) Name() string { return f.name }
func (f *fakeNotifier) Deliver(_ context.Context, _ storage.Alert) error {
	f.calls.Add(1)
	left := atomic.LoadInt32(&f.failures)
	if left > 0 {
		atomic.AddInt32(&f.failures, -1)
		if f.permanent {
			return fmt.Errorf("%w: nope", ErrPermanent)
		}
		return errors.New("transient")
	}
	return nil
}

func TestDispatcher_SucceedsFirstTry(t *testing.T) {
	t.Parallel()
	n := &fakeNotifier{name: "ok"}
	dlq := NewMemoryDeadLetterStore(10)
	d := NewDispatcher([]Notifier{n}, dlq, RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond}, nil)
	d.Dispatch(context.Background(), storage.Alert{ID: "a1"})
	if n.calls.Load() != 1 {
		t.Fatalf("expected 1 call, got %d", n.calls.Load())
	}
	got, _ := dlq.ListDeadLetters(context.Background(), 10)
	if len(got) != 0 {
		t.Fatalf("expected 0 dead letters, got %d", len(got))
	}
	d1, _, dl := d.Metrics()
	if d1 != 1 || dl != 0 {
		t.Fatalf("metrics: delivered=%d dead=%d", d1, dl)
	}
}

func TestDispatcher_RetriesThenSucceeds(t *testing.T) {
	t.Parallel()
	n := &fakeNotifier{name: "flaky", failures: 2}
	dlq := NewMemoryDeadLetterStore(10)
	d := NewDispatcher([]Notifier{n}, dlq, RetryPolicy{MaxAttempts: 5, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond}, nil)
	d.Dispatch(context.Background(), storage.Alert{ID: "a2"})
	if n.calls.Load() != 3 {
		t.Fatalf("expected 3 calls (2 fails + 1 success), got %d", n.calls.Load())
	}
	got, _ := dlq.ListDeadLetters(context.Background(), 10)
	if len(got) != 0 {
		t.Fatalf("expected 0 dead letters, got %d", len(got))
	}
}

func TestDispatcher_DeadLettersAfterMaxAttempts(t *testing.T) {
	t.Parallel()
	n := &fakeNotifier{name: "broken", failures: 99}
	dlq := NewMemoryDeadLetterStore(10)
	d := NewDispatcher([]Notifier{n}, dlq, RetryPolicy{MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond}, nil)
	d.Dispatch(context.Background(), storage.Alert{ID: "a3"})
	if n.calls.Load() != 3 {
		t.Fatalf("expected 3 calls, got %d", n.calls.Load())
	}
	got, _ := dlq.ListDeadLetters(context.Background(), 10)
	if len(got) != 1 {
		t.Fatalf("expected 1 dead letter, got %d", len(got))
	}
	if got[0].AlertID != "a3" {
		t.Fatalf("dead letter alertID = %q", got[0].AlertID)
	}
	if got[0].Notifier != "broken" {
		t.Fatalf("dead letter notifier = %q", got[0].Notifier)
	}
}

func TestDispatcher_PermanentSkipsRetries(t *testing.T) {
	t.Parallel()
	n := &fakeNotifier{name: "perm", failures: 99, permanent: true}
	dlq := NewMemoryDeadLetterStore(10)
	d := NewDispatcher([]Notifier{n}, dlq, RetryPolicy{MaxAttempts: 5, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond}, nil)
	d.Dispatch(context.Background(), storage.Alert{ID: "a4"})
	if got := n.calls.Load(); got != 1 {
		t.Fatalf("expected 1 call (no retry on permanent), got %d", got)
	}
	got, _ := dlq.ListDeadLetters(context.Background(), 10)
	if len(got) != 1 {
		t.Fatalf("expected 1 dead letter, got %d", len(got))
	}
}

func TestDispatcher_FansOutToAllNotifiers(t *testing.T) {
	t.Parallel()
	a := &fakeNotifier{name: "a"}
	b := &fakeNotifier{name: "b"}
	c := &fakeNotifier{name: "c", failures: 99}
	dlq := NewMemoryDeadLetterStore(10)
	d := NewDispatcher([]Notifier{a, b, c}, dlq, RetryPolicy{MaxAttempts: 2, BaseDelay: time.Millisecond, MaxDelay: 2 * time.Millisecond}, nil)
	d.Dispatch(context.Background(), storage.Alert{ID: "fanout"})
	if a.calls.Load() != 1 || b.calls.Load() != 1 {
		t.Fatalf("expected one call each to a,b; got a=%d b=%d", a.calls.Load(), b.calls.Load())
	}
	if c.calls.Load() != 2 {
		t.Fatalf("expected 2 calls to c, got %d", c.calls.Load())
	}
	got, _ := dlq.ListDeadLetters(context.Background(), 10)
	if len(got) != 1 || got[0].Notifier != "c" {
		t.Fatalf("expected only c dead-lettered, got %+v", got)
	}
}

func TestDispatcher_ContextCancelStopsRetries(t *testing.T) {
	t.Parallel()
	n := &fakeNotifier{name: "slow", failures: 99}
	dlq := NewMemoryDeadLetterStore(10)
	d := NewDispatcher([]Notifier{n}, dlq, RetryPolicy{MaxAttempts: 99, BaseDelay: 200 * time.Millisecond, MaxDelay: time.Second}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		d.Dispatch(ctx, storage.Alert{ID: "cancel"})
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	wg.Wait()
	if n.calls.Load() == 0 {
		t.Fatalf("expected at least one call before cancel")
	}
	if n.calls.Load() > 5 {
		t.Fatalf("expected cancel to stop retries quickly, saw %d calls", n.calls.Load())
	}
}

func TestWebhookNotifier_HappyPath(t *testing.T) {
	t.Parallel()
	var got struct {
		gotBody bool
		mu      sync.Mutex
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.mu.Lock()
		got.gotBody = r.ContentLength > 0
		got.mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()
	wh := &WebhookNotifier{URLStr: srv.URL}
	err := wh.Deliver(context.Background(), storage.Alert{ID: "w1", Headline: "hi"})
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	got.mu.Lock()
	defer got.mu.Unlock()
	if !got.gotBody {
		t.Fatalf("expected webhook to receive a non-empty body")
	}
}

func TestWebhookNotifier_4xxIsPermanent(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer srv.Close()
	wh := &WebhookNotifier{URLStr: srv.URL}
	err := wh.Deliver(context.Background(), storage.Alert{ID: "w2"})
	if err == nil || !errors.Is(err, ErrPermanent) {
		t.Fatalf("expected ErrPermanent, got %v", err)
	}
}

func TestWebhookNotifier_5xxIsTransient(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	wh := &WebhookNotifier{URLStr: srv.URL}
	err := wh.Deliver(context.Background(), storage.Alert{ID: "w3"})
	if err == nil || errors.Is(err, ErrPermanent) {
		t.Fatalf("expected transient, got %v", err)
	}
}

func TestMemoryDLQ_EvictsOldest(t *testing.T) {
	t.Parallel()
	dlq := NewMemoryDeadLetterStore(2)
	for i := 0; i < 5; i++ {
		_ = dlq.InsertDeadLetter(context.Background(), DeadLetter{AlertID: fmt.Sprintf("a%d", i)})
	}
	got, _ := dlq.ListDeadLetters(context.Background(), 10)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	if got[0].AlertID != "a4" || got[1].AlertID != "a3" {
		t.Fatalf("expected newest-first (a4, a3), got %s, %s", got[0].AlertID, got[1].AlertID)
	}
}

func TestFileDLQ_PersistsAcrossOpens(t *testing.T) {
	t.Parallel()
	path := t.TempDir() + "/dlq.jsonl"
	s, err := NewFileDeadLetterStore(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		_ = s.InsertDeadLetter(context.Background(), DeadLetter{AlertID: fmt.Sprintf("a%d", i)})
	}
	s2, err := NewFileDeadLetterStore(path)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := s2.ListDeadLetters(context.Background(), 10)
	if len(got) != 3 {
		t.Fatalf("expected 3 persisted entries, got %d", len(got))
	}
	if got[0].AlertID != "a2" {
		t.Fatalf("expected newest first (a2), got %s", got[0].AlertID)
	}
}

func TestBackoff_RespectsMax(t *testing.T) {
	t.Parallel()
	if d := backoff(time.Second, 5*time.Second, 10); d != 5*time.Second {
		t.Fatalf("backoff overshoot: %v", d)
	}
	if d := backoff(time.Second, 5*time.Second, 1); d != time.Second {
		t.Fatalf("attempt 1 = %v", d)
	}
	if d := backoff(time.Second, 5*time.Second, 2); d != 2*time.Second {
		t.Fatalf("attempt 2 = %v", d)
	}
	if d := backoff(time.Second, 5*time.Second, 3); d != 4*time.Second {
		t.Fatalf("attempt 3 = %v", d)
	}
}
