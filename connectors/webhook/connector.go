// Package webhook is a generic ModePush connector for Sunny.
//
// The runtime mounts each instance at:
//
//	POST /api/ingest/<instance-id>/
//	POST /api/ingest/<instance-id>/<anything>
//
// The request body is captured as the record's payload. Optional headers
// drive metadata:
//
//	X-Sunny-Source-Id     populates record.SourceID
//	X-Sunny-Tag-<key>     each "X-Sunny-Tag-Foo: bar" becomes tags[foo] = "bar"
//	X-Sunny-Lat / X-Sunny-Lng / X-Sunny-Altitude   populate record.Location
//	X-Sunny-Timestamp     RFC3339; defaults to receive time
//
// If the request includes a header `X-Sunny-Token`, it must match the
// `requireToken` config value (or `SUNNY_SECRET_WEBHOOK_<INSTANCE_UPPER>`).
package webhook

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	sdk "github.com/sunny/sunny/packages/sdk-go"
)

const (
	ID      = "webhook"
	Version = "0.1.0"
)

type Config struct {
	// RequireToken: if non-empty, requests must carry X-Sunny-Token: <value>.
	// You can also set the token via the secret named "webhook-token-<instance>"
	// for cleaner config — secret value wins if both are set.
	RequireToken string `json:"requireToken"`

	// MaxBodyBytes caps how many bytes we read per request. Default 256 KiB.
	MaxBodyBytes int64 `json:"maxBodyBytes"`
}

type Connector struct{}

func New() sdk.Connector { return &Connector{} }

func (Connector) Manifest() sdk.Manifest {
	return sdk.Manifest{
		ID:          ID,
		Name:        "Generic Webhook",
		Version:     Version,
		Category:    sdk.CategoryCustom,
		Mode:        sdk.ModePush,
		Description: "Receives JSON via POST /api/ingest/<instance-id>/. Tags via X-Sunny-Tag-* headers.",
		ConfigSchema: json.RawMessage(`{
			"type": "object",
			"properties": {
				"requireToken": {"type": "string", "description": "if set, requests must send X-Sunny-Token: <value>"},
				"maxBodyBytes": {"type": "integer", "default": 262144}
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
		return fmt.Errorf("webhook config: %w", err)
	}
	if c.MaxBodyBytes < 0 {
		return errors.New("maxBodyBytes must be >= 0")
	}
	return nil
}

// Run for a pure-push connector: just block until cancelled. The actual
// work happens inside the http.Handler returned by BuildPushHandler.
func (Connector) Run(ctx context.Context, _ sdk.Context, _ json.RawMessage) error {
	<-ctx.Done()
	return ctx.Err()
}

// BuildPushHandler implements sdk.PushHandler.
func (Connector) BuildPushHandler(rt sdk.Context, raw json.RawMessage) (http.Handler, error) {
	cfg := Config{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return nil, err
		}
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 256 * 1024
	}

	expectedToken := cfg.RequireToken
	if t := rt.Secret("webhook-token"); t != "" {
		expectedToken = t
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost && r.Method != http.MethodPut {
			http.Error(w, "use POST or PUT", http.StatusMethodNotAllowed)
			return
		}
		if expectedToken != "" {
			got := r.Header.Get("X-Sunny-Token")
			if got != expectedToken {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		body, err := io.ReadAll(io.LimitReader(r.Body, cfg.MaxBodyBytes))
		if err != nil {
			http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
			return
		}
		_ = r.Body.Close()

		// Validate JSON; if not JSON, wrap as {"raw": "<body>"}.
		var payload json.RawMessage
		if json.Valid(body) {
			payload = body
		} else {
			b, _ := json.Marshal(map[string]string{"raw": string(body)})
			payload = b
		}

		ts := time.Now().UTC()
		if h := r.Header.Get("X-Sunny-Timestamp"); h != "" {
			if parsed, err := time.Parse(time.RFC3339, h); err == nil {
				ts = parsed.UTC()
			}
		}

		var loc *sdk.GeoPoint
		latS := r.Header.Get("X-Sunny-Lat")
		lngS := r.Header.Get("X-Sunny-Lng")
		if latS != "" && lngS != "" {
			lat, errLat := strconv.ParseFloat(latS, 64)
			lng, errLng := strconv.ParseFloat(lngS, 64)
			if errLat == nil && errLng == nil {
				loc = &sdk.GeoPoint{Lat: lat, Lng: lng}
				if altS := r.Header.Get("X-Sunny-Altitude"); altS != "" {
					if alt, err := strconv.ParseFloat(altS, 64); err == nil {
						loc.Altitude = &alt
					}
				}
			}
		}

		tags := map[string]string{}
		for k, vs := range r.Header {
			if strings.HasPrefix(k, "X-Sunny-Tag-") && len(vs) > 0 {
				tags[strings.ToLower(strings.TrimPrefix(k, "X-Sunny-Tag-"))] = vs[0]
			}
		}

		rec := sdk.Record{
			Timestamp: ts,
			SourceID:  r.Header.Get("X-Sunny-Source-Id"),
			Location:  loc,
			Tags:      tags,
			Payload:   payload,
		}
		if err := rt.Publish(r.Context(), rec); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"status":"accepted"}`)
	}), nil
}
