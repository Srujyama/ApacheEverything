// Package postgres is a Sunny stream-mode connector for Postgres
// LISTEN/NOTIFY.
//
// Each NOTIFY arriving on a configured channel becomes a Sunny record.
// The notification's payload (Postgres allows up to ~8KB strings) becomes
// the record payload — JSON if it parses as JSON, otherwise wrapped as
// {"raw": "..."}. The channel name is stamped into tags["channel"].
//
// Connection string is the standard libpq DSN
// (postgres://user:pass@host:5432/db?sslmode=disable). Username/password
// can also come from secrets: SUNNY_SECRET_POSTGRES_USERNAME,
// SUNNY_SECRET_POSTGRES_PASSWORD. If both DSN credentials and secrets are
// set, secrets win.
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"
	sdk "github.com/sunny/sunny/packages/sdk-go"
)

const (
	ID      = "postgres"
	Version = "0.1.0"
)

type Config struct {
	// DSN: postgres://user:pass@host:5432/dbname?sslmode=disable
	DSN string `json:"dsn"`

	// Channels to LISTEN on. Required.
	Channels []string `json:"channels"`

	// ReconnectSeconds is how long to wait before reconnecting after a
	// dropped connection. Default 5.
	ReconnectSeconds int `json:"reconnectSeconds"`
}

func (c *Config) applyDefaults() {
	if c.ReconnectSeconds <= 0 {
		c.ReconnectSeconds = 5
	}
}

type Connector struct{}

func New() sdk.Connector { return &Connector{} }

func (Connector) Manifest() sdk.Manifest {
	return sdk.Manifest{
		ID:          ID,
		Name:        "Postgres LISTEN/NOTIFY",
		Version:     Version,
		Category:    sdk.CategoryIoT,
		Mode:        sdk.ModeStream,
		Description: "Receives Postgres NOTIFY events as records. Reference for change-data-capture flows.",
		ConfigSchema: json.RawMessage(`{
			"type": "object",
			"required": ["dsn", "channels"],
			"properties": {
				"dsn": {"type": "string", "description": "postgres://user:pass@host:5432/db?sslmode=disable"},
				"channels": {"type": "array", "items": {"type": "string"}, "minItems": 1},
				"reconnectSeconds": {"type": "integer", "minimum": 1, "default": 5}
			}
		}`),
	}
}

func (Connector) Validate(raw json.RawMessage) error {
	if len(raw) == 0 {
		return errors.New("postgres requires config: dsn and channels")
	}
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return fmt.Errorf("postgres config: %w", err)
	}
	if c.DSN == "" {
		return errors.New("dsn is required")
	}
	if len(c.Channels) == 0 {
		return errors.New("at least one channel is required")
	}
	for _, ch := range c.Channels {
		if ch == "" {
			return errors.New("channel name cannot be empty")
		}
	}
	return nil
}

func (Connector) Run(ctx context.Context, rt sdk.Context, raw json.RawMessage) error {
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}
	cfg.applyDefaults()

	dsn, err := overrideCredentials(cfg.DSN, rt)
	if err != nil {
		return fmt.Errorf("postgres dsn: %w", err)
	}

	rt.Logger().Info("postgres starting", "channels", cfg.Channels)

	for {
		if err := listen(ctx, rt, dsn, cfg.Channels); err != nil && !errors.Is(err, context.Canceled) {
			rt.Logger().Warn("postgres connection error", "err", err)
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(cfg.ReconnectSeconds) * time.Second):
		}
	}
}

// overrideCredentials replaces the DSN's user/password with secret values
// if the operator set them. Returns the (possibly modified) DSN.
func overrideCredentials(dsn string, rt sdk.Context) (string, error) {
	user := rt.Secret("postgres-username")
	pass := rt.Secret("postgres-password")
	if user == "" && pass == "" {
		return dsn, nil
	}
	u, err := url.Parse(dsn)
	if err != nil {
		return "", err
	}
	existingUser := ""
	if u.User != nil {
		existingUser = u.User.Username()
	}
	if user == "" {
		user = existingUser
	}
	if pass != "" {
		u.User = url.UserPassword(user, pass)
	} else {
		u.User = url.User(user)
	}
	return u.String(), nil
}

// listen opens one connection, subscribes to each channel, and forwards
// notifications until the context is cancelled or the connection drops.
func listen(ctx context.Context, rt sdk.Context, dsn string, channels []string) error {
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())

	for _, ch := range channels {
		// Identifiers can't be parameterized in LISTEN; the channel must be a
		// trusted string. We require it to be non-empty (validated upstream).
		if _, err := conn.Exec(ctx, "LISTEN "+pgxQuoteIdent(ch)); err != nil {
			return fmt.Errorf("LISTEN %s: %w", ch, err)
		}
	}
	rt.Logger().Info("postgres connected", "channels", channels)

	for {
		notif, err := conn.WaitForNotification(ctx)
		if err != nil {
			return err
		}
		var payload json.RawMessage
		if json.Valid([]byte(notif.Payload)) {
			payload = json.RawMessage(notif.Payload)
		} else {
			b, _ := json.Marshal(map[string]string{"raw": notif.Payload})
			payload = b
		}
		_ = rt.Publish(ctx, sdk.Record{
			Timestamp: time.Now().UTC(),
			SourceID:  fmt.Sprintf("%s#%d", notif.Channel, notif.PID),
			Tags: map[string]string{
				"channel": notif.Channel,
				"pid":     fmt.Sprintf("%d", notif.PID),
			},
			Payload: payload,
		})
	}
}

// pgxQuoteIdent wraps an identifier in double-quotes and escapes embedded
// quotes per Postgres rules. Equivalent to `pq.QuoteIdentifier`.
func pgxQuoteIdent(s string) string {
	out := []byte{'"'}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '"' {
			out = append(out, '"', '"')
		} else {
			out = append(out, c)
		}
	}
	out = append(out, '"')
	return string(out)
}
