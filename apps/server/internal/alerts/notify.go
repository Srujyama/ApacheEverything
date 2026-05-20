// notify.go is the alert delivery subsystem.
//
// When the rule engine fires an alert, the alert is (1) persisted via
// storage.InsertAlert and (2) handed to the Dispatcher. The Dispatcher fans
// out to all configured Notifiers. Each notifier attempts delivery; failures
// are retried with exponential backoff. Alerts that exhaust retries land in
// the dead-letter store for manual inspection.
//
// This file ships three notifier implementations:
//
//   - WebhookNotifier: HTTP POST a JSON payload to a URL.
//   - SlackNotifier:   POST a Slack-formatted message to an incoming webhook.
//   - LogNotifier:     write to the structured log (default for embedded mode).
//
// Email is intentionally NOT bundled here — SMTP config sprawl is wide
// enough that we want it as a separate package once Phase 0.8 ships the
// CLI knobs for it.
package alerts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sunny/sunny/apps/server/internal/storage"
)

// Notifier delivers a single alert. Returning a non-nil error means the
// dispatcher should retry; returning an error wrapped with ErrPermanent
// (errors.Is) means the dispatcher should skip retries and dead-letter.
type Notifier interface {
	// Name identifies the notifier in logs and metrics. Must be unique
	// across the dispatcher's configured notifiers.
	Name() string

	// Deliver sends the alert. Implementations MUST honor ctx cancellation.
	Deliver(ctx context.Context, a storage.Alert) error
}

// ErrPermanent marks delivery failures that retrying won't fix (e.g., 400-class
// HTTP responses, malformed configuration). Wrap it with %w.
var ErrPermanent = errors.New("permanent notifier failure")

// DeadLetterStore is the persistence contract for alerts that exhausted
// retries. It is intentionally narrow so any storage backend can implement
// it cheaply. In v0.1 we satisfy it via the existing duckdb-backed Backend
// (see deadletter.go).
type DeadLetterStore interface {
	InsertDeadLetter(ctx context.Context, dl DeadLetter) error
	ListDeadLetters(ctx context.Context, limit int) ([]DeadLetter, error)
}

// DeadLetter is one row in the DLQ.
type DeadLetter struct {
	ID         string          `json:"id"`
	AlertID    string          `json:"alertId"`
	Notifier   string          `json:"notifier"`
	Reason     string          `json:"reason"`
	Attempts   int             `json:"attempts"`
	LastTried  time.Time       `json:"lastTried"`
	AlertJSON  json.RawMessage `json:"alert"`
}

// RetryPolicy controls backoff behavior.
type RetryPolicy struct {
	MaxAttempts int           // total tries before dead-lettering (must be >= 1)
	BaseDelay   time.Duration // first retry delay
	MaxDelay    time.Duration // cap on per-attempt delay
}

// DefaultRetryPolicy is sized for the typical webhook target: handful of
// retries, escalating gently, capped well under our 30s request timeout.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{MaxAttempts: 5, BaseDelay: time.Second, MaxDelay: 30 * time.Second}
}

// Dispatcher fans an alert out to every configured notifier and applies the
// retry policy. It is safe for concurrent use.
type Dispatcher struct {
	notifiers []Notifier
	dlq       DeadLetterStore
	policy    RetryPolicy
	logger    *slog.Logger

	// metrics
	delivered atomic.Uint64
	retried   atomic.Uint64
	deadLet   atomic.Uint64
}

// NewDispatcher wires notifiers to the dead-letter store. A nil dlq is
// allowed (the dispatcher will just log dead letters in that case), but
// production callers should always pass one.
func NewDispatcher(notifiers []Notifier, dlq DeadLetterStore, policy RetryPolicy, logger *slog.Logger) *Dispatcher {
	if policy.MaxAttempts <= 0 {
		policy.MaxAttempts = 1
	}
	if policy.BaseDelay <= 0 {
		policy.BaseDelay = time.Second
	}
	if policy.MaxDelay <= 0 {
		policy.MaxDelay = 30 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &Dispatcher{notifiers: notifiers, dlq: dlq, policy: policy, logger: logger}
}

// Dispatch hands the alert to every notifier. Each notifier is retried
// independently. The call returns once all notifiers have either succeeded
// or been dead-lettered; failures are surfaced via metrics, not return value.
func (d *Dispatcher) Dispatch(ctx context.Context, a storage.Alert) {
	if len(d.notifiers) == 0 {
		return
	}
	var wg sync.WaitGroup
	for _, n := range d.notifiers {
		wg.Add(1)
		go func(n Notifier) {
			defer wg.Done()
			d.deliverWithRetry(ctx, n, a)
		}(n)
	}
	wg.Wait()
}

// Metrics returns a snapshot of dispatcher counters.
func (d *Dispatcher) Metrics() (delivered, retried, deadLettered uint64) {
	return d.delivered.Load(), d.retried.Load(), d.deadLet.Load()
}

func (d *Dispatcher) deliverWithRetry(ctx context.Context, n Notifier, a storage.Alert) {
	var lastErr error
	for attempt := 1; attempt <= d.policy.MaxAttempts; attempt++ {
		err := n.Deliver(ctx, a)
		if err == nil {
			d.delivered.Add(1)
			return
		}
		lastErr = err
		if errors.Is(err, ErrPermanent) || ctx.Err() != nil {
			break
		}
		d.retried.Add(1)
		delay := backoff(d.policy.BaseDelay, d.policy.MaxDelay, attempt)
		d.logger.Warn("alert notifier transient failure; retrying",
			"notifier", n.Name(), "alert", a.ID, "attempt", attempt, "delay", delay, "err", err)
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
	}
	d.deadLet.Add(1)
	d.logger.Error("alert notifier exhausted; dead-lettering",
		"notifier", n.Name(), "alert", a.ID, "attempts", d.policy.MaxAttempts, "err", lastErr)
	if d.dlq != nil {
		body, _ := json.Marshal(a)
		_ = d.dlq.InsertDeadLetter(ctx, DeadLetter{
			ID:        a.ID + ":" + n.Name(),
			AlertID:   a.ID,
			Notifier:  n.Name(),
			Reason:    lastErr.Error(),
			Attempts:  d.policy.MaxAttempts,
			LastTried: time.Now().UTC(),
			AlertJSON: body,
		})
	}
}

// backoff returns base * 2^(attempt-1), clamped to max.
func backoff(base, max time.Duration, attempt int) time.Duration {
	d := base
	for i := 1; i < attempt; i++ {
		d *= 2
		if d > max {
			return max
		}
	}
	if d > max {
		return max
	}
	return d
}

// ---------------------------------------------------------------------------
// Built-in notifiers
// ---------------------------------------------------------------------------

// LogNotifier writes alerts to slog. Always succeeds. Default notifier for
// installations that haven't configured anything else.
type LogNotifier struct {
	Logger *slog.Logger
}

func (l *LogNotifier) Name() string { return "log" }
func (l *LogNotifier) Deliver(_ context.Context, a storage.Alert) error {
	lg := l.Logger
	if lg == nil {
		lg = slog.Default()
	}
	lg.Info("alert", "id", a.ID, "rule", a.RuleName, "severity", a.Severity,
		"connector", a.ConnectorID, "source", a.SourceID, "headline", a.Headline)
	return nil
}

// WebhookNotifier POSTs the alert JSON to a URL.
type WebhookNotifier struct {
	URLStr  string
	Headers map[string]string
	Client  *http.Client // nil → http.DefaultClient with 10s timeout
}

func (w *WebhookNotifier) Name() string { return "webhook:" + w.URLStr }

func (w *WebhookNotifier) Deliver(ctx context.Context, a storage.Alert) error {
	if w.URLStr == "" {
		return fmt.Errorf("%w: empty webhook URL", ErrPermanent)
	}
	body, err := json.Marshal(a)
	if err != nil {
		return fmt.Errorf("%w: marshal alert: %v", ErrPermanent, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.URLStr, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: build request: %v", ErrPermanent, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "sunny-alerts/1")
	for k, v := range w.Headers {
		req.Header.Set(k, v)
	}
	c := w.Client
	if c == nil {
		c = &http.Client{Timeout: 10 * time.Second}
	}
	res, err := c.Do(req)
	if err != nil {
		return err // transient
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode >= 200 && res.StatusCode < 300:
		return nil
	case res.StatusCode >= 400 && res.StatusCode < 500:
		// 4xx = our request is wrong. Retrying won't help.
		return fmt.Errorf("%w: %s", ErrPermanent, res.Status)
	default:
		return fmt.Errorf("delivery failed: %s", res.Status)
	}
}

// SlackNotifier is a thin wrapper over WebhookNotifier that formats the body
// as Slack expects.
type SlackNotifier struct {
	WebhookURL string
	Channel    string // optional override
	Client     *http.Client
}

func (s *SlackNotifier) Name() string { return "slack" }

func (s *SlackNotifier) Deliver(ctx context.Context, a storage.Alert) error {
	if s.WebhookURL == "" {
		return fmt.Errorf("%w: empty slack webhook URL", ErrPermanent)
	}
	payload := map[string]any{
		"text": fmt.Sprintf("*[%s]* %s — %s/%s",
			emojiForSeverity(a.Severity), a.Headline, a.ConnectorID, a.SourceID),
		"attachments": []map[string]any{{
			"color":  colorForSeverity(a.Severity),
			"fields": slackFields(a),
			"footer": "sunny",
			"ts":     a.Triggered.Unix(),
		}},
	}
	if s.Channel != "" {
		payload["channel"] = s.Channel
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("%w: marshal slack body: %v", ErrPermanent, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("%w: build request: %v", ErrPermanent, err)
	}
	req.Header.Set("Content-Type", "application/json")
	c := s.Client
	if c == nil {
		c = &http.Client{Timeout: 10 * time.Second}
	}
	res, err := c.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}
	if res.StatusCode >= 400 && res.StatusCode < 500 {
		return fmt.Errorf("%w: slack %s", ErrPermanent, res.Status)
	}
	return fmt.Errorf("slack delivery failed: %s", res.Status)
}

func slackFields(a storage.Alert) []map[string]any {
	out := []map[string]any{
		{"title": "Rule", "value": a.RuleName, "short": true},
		{"title": "Severity", "value": defaultStr(a.Severity, "n/a"), "short": true},
		{"title": "Connector", "value": a.ConnectorID, "short": true},
		{"title": "Source", "value": defaultStr(a.SourceID, "—"), "short": true},
	}
	for k, v := range a.Tags {
		out = append(out, map[string]any{"title": k, "value": v, "short": true})
	}
	return out
}

func emojiForSeverity(s string) string {
	switch s {
	case "emergency":
		return "rotating_light"
	case "critical":
		return "warning"
	case "warning":
		return "exclamation"
	default:
		return "information_source"
	}
}

func colorForSeverity(s string) string {
	switch s {
	case "emergency", "critical":
		return "#d62728"
	case "warning":
		return "#ff9900"
	default:
		return "#1f77b4"
	}
}

func defaultStr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
