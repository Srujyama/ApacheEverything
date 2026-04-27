// Package alerts is the rule engine. It subscribes to the bus, evaluates
// every record against the active rule set, and persists triggered alerts.
//
// Rules are stored in DuckDB so they survive restarts. The engine reloads
// rules periodically (every 30s) so changes via the HTTP API take effect
// without bouncing the server.
package alerts

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"github.com/sunny/sunny/apps/server/internal/bus"
	"github.com/sunny/sunny/apps/server/internal/storage"
	sdk "github.com/sunny/sunny/packages/sdk-go"
)

// Engine evaluates rules against records published on the bus.
type Engine struct {
	bus    *bus.Bus
	store  storage.Storage
	logger *slog.Logger

	mu    sync.RWMutex
	rules []storage.AlertRule

	// dedupe ring: prevents re-emitting the same alert for the same
	// (rule, source) within RuleDedupeWindow when a connector republishes.
	dedupe       map[string]time.Time
	dedupeWindow time.Duration
}

// New constructs an Engine. Caller is responsible for calling Run/Close.
func New(b *bus.Bus, store storage.Storage, logger *slog.Logger) *Engine {
	return &Engine{
		bus:          b,
		store:        store,
		logger:       logger,
		dedupe:       map[string]time.Time{},
		dedupeWindow: 5 * time.Minute,
	}
}

// SeedDefaultRule installs a critical-severity rule if no rules exist yet.
// Called once on startup so a fresh install has something firing.
func (e *Engine) SeedDefaultRule(ctx context.Context) error {
	rules, err := e.store.ListRules(ctx)
	if err != nil {
		return err
	}
	if len(rules) > 0 {
		return nil
	}
	r := storage.AlertRule{
		ID:         "default-critical",
		Name:       "Critical and emergency severity",
		Enabled:    true,
		SeverityIn: []string{"emergency", "critical"},
		CreatedAt:  time.Now().UTC(),
	}
	return e.store.SaveRule(ctx, r)
}

// Run subscribes to the bus and evaluates records until ctx is cancelled.
func (e *Engine) Run(ctx context.Context) error {
	if err := e.reloadRules(ctx); err != nil {
		e.logger.Warn("alerts: initial rule load", "err", err)
	}

	sub := e.bus.Subscribe(nil, false)
	defer sub.Close()

	reloadTick := time.NewTicker(30 * time.Second)
	defer reloadTick.Stop()
	pruneTick := time.NewTicker(time.Minute)
	defer pruneTick.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-reloadTick.C:
			if err := e.reloadRules(ctx); err != nil {
				e.logger.Warn("alerts: reload", "err", err)
			}
		case <-pruneTick.C:
			e.pruneDedupe()
		case rec, ok := <-sub.C():
			if !ok {
				return nil
			}
			e.evaluate(ctx, rec)
		}
	}
}

// Rules returns a copy of the current ruleset.
func (e *Engine) Rules() []storage.AlertRule {
	e.mu.RLock()
	defer e.mu.RUnlock()
	out := make([]storage.AlertRule, len(e.rules))
	copy(out, e.rules)
	return out
}

func (e *Engine) reloadRules(ctx context.Context) error {
	rs, err := e.store.ListRules(ctx)
	if err != nil {
		return err
	}
	e.mu.Lock()
	e.rules = rs
	e.mu.Unlock()
	return nil
}

func (e *Engine) pruneDedupe() {
	e.mu.Lock()
	defer e.mu.Unlock()
	cut := time.Now().Add(-e.dedupeWindow)
	for k, t := range e.dedupe {
		if t.Before(cut) {
			delete(e.dedupe, k)
		}
	}
}

func (e *Engine) evaluate(ctx context.Context, r sdk.Record) {
	e.mu.RLock()
	rules := e.rules
	e.mu.RUnlock()

	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if !matches(rule, r) {
			continue
		}
		key := rule.ID + "|" + r.ConnectorID + "|" + r.SourceID
		e.mu.Lock()
		if last, ok := e.dedupe[key]; ok && time.Since(last) < e.dedupeWindow {
			e.mu.Unlock()
			continue
		}
		e.dedupe[key] = time.Now()
		e.mu.Unlock()

		alert := storage.Alert{
			ID:          newID(),
			RuleID:      rule.ID,
			RuleName:    rule.Name,
			ConnectorID: r.ConnectorID,
			SourceID:    r.SourceID,
			Severity:    severityOf(r),
			Headline:    headlineOf(r),
			Tags:        r.Tags,
			Payload:     r.Payload,
			Triggered:   time.Now().UTC(),
		}
		if err := e.store.InsertAlert(ctx, alert); err != nil {
			e.logger.Warn("alerts: insert", "err", err)
		}
	}
}

func matches(rule storage.AlertRule, r sdk.Record) bool {
	if rule.ConnectorID != "" && rule.ConnectorID != r.ConnectorID {
		return false
	}
	if len(rule.SeverityIn) > 0 {
		s := severityOf(r)
		ok := false
		for _, want := range rule.SeverityIn {
			if want == s {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	for k, v := range rule.TagEquals {
		if r.Tags == nil || r.Tags[k] != v {
			return false
		}
	}
	return true
}

func severityOf(r sdk.Record) string {
	if r.Tags == nil {
		return ""
	}
	return r.Tags["severity"]
}

func headlineOf(r sdk.Record) string {
	// Try common payload fields used by our connectors. Fall back to source id.
	type payloadShape struct {
		Headline  string `json:"headline"`
		Event     string `json:"event"`
		Place     string `json:"place"`
		SiteName  string `json:"siteName"`
		Parameter string `json:"parameter"`
	}
	if len(r.Payload) > 0 {
		var p payloadShape
		if err := json.Unmarshal(r.Payload, &p); err == nil {
			switch {
			case p.Headline != "":
				return p.Headline
			case p.Event != "":
				return p.Event
			case p.Place != "":
				return p.Place
			case p.SiteName != "":
				return p.SiteName
			case p.Parameter != "":
				return p.Parameter
			}
		}
	}
	if r.SourceID != "" {
		return r.ConnectorID + ": " + r.SourceID
	}
	return r.ConnectorID
}

func newID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
