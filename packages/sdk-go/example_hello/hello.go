// Package hello is the canonical "hello world" connector used in docs and
// SDK tests. It emits one record on a configurable interval (default 2s).
//
// This package intentionally does NOT register itself with the runtime —
// the runtime lives in apps/server, which would create an import cycle.
// The server's connectors_register.go does the wiring.
package hello

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	sdk "github.com/sunny/sunny/packages/sdk-go"
)

const (
	ID      = "hello"
	version = "0.1.0"
)

// Config is the user-supplied configuration for a hello instance.
type Config struct {
	// IntervalSeconds is how often to emit a record. Defaults to 2.
	IntervalSeconds int `json:"intervalSeconds"`
	// Greeting is the string included in each record's payload. Defaults to "hello".
	Greeting string `json:"greeting"`
}

func (c *Config) applyDefaults() {
	if c.IntervalSeconds <= 0 {
		c.IntervalSeconds = 2
	}
	if c.Greeting == "" {
		c.Greeting = "hello"
	}
}

type Connector struct{}

// New returns a new instance of the connector. Use this when registering.
func New() sdk.Connector { return Connector{} }

func (Connector) Manifest() sdk.Manifest {
	return sdk.Manifest{
		ID:          ID,
		Name:        "Hello World",
		Version:     version,
		Category:    sdk.CategoryCustom,
		Mode:        sdk.ModePull,
		Description: "Emits a fake observation on a fixed interval. Used to verify the runtime.",
		ConfigSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"intervalSeconds": {"type": "integer", "minimum": 1, "default": 2},
				"greeting": {"type": "string", "default": "hello"}
			}
		}`),
	}
}

func (Connector) Validate(raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return fmt.Errorf("invalid hello config: %w", err)
	}
	if c.IntervalSeconds < 0 {
		return fmt.Errorf("intervalSeconds must be >= 0, got %d", c.IntervalSeconds)
	}
	return nil
}

func (Connector) Run(ctx context.Context, rt sdk.Context, raw json.RawMessage) error {
	cfg := Config{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return err
		}
	}
	cfg.applyDefaults()

	rt.Logger().Info("hello connector starting", "intervalSeconds", cfg.IntervalSeconds)
	tick := time.NewTicker(time.Duration(cfg.IntervalSeconds) * time.Second)
	defer tick.Stop()

	var count int64
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case t := <-tick.C:
			count++
			payload, _ := json.Marshal(map[string]any{
				"greeting": cfg.Greeting,
				"count":    count,
			})
			if err := rt.Publish(ctx, sdk.Record{
				Timestamp: t.UTC(),
				Payload:   payload,
			}); err != nil {
				return err
			}
		}
	}
}
