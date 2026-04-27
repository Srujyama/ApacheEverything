// Package mqtt is a Sunny stream-mode connector for MQTT brokers.
//
// Connects to a configured MQTT broker, subscribes to one or more topic
// patterns, and publishes each message as a Sunny record. The message
// payload becomes the record payload (raw bytes wrapped if not JSON).
// The MQTT topic is stamped into tags["topic"].
//
// Authentication: optional username/password from config, or
// SUNNY_SECRET_MQTT_USERNAME / SUNNY_SECRET_MQTT_PASSWORD env vars.
package mqtt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	mqttp "github.com/eclipse/paho.mqtt.golang"
	sdk "github.com/sunny/sunny/packages/sdk-go"
)

const (
	ID      = "mqtt"
	Version = "0.1.0"
)

type Config struct {
	// Broker URL: tcp://host:1883, ssl://host:8883, ws://host:8080.
	Broker string `json:"broker"`

	// Topic patterns to subscribe to. Wildcards (+, #) supported.
	Topics []string `json:"topics"`

	// QoS for subscriptions. 0/1/2. Default 1 (at least once).
	QoS byte `json:"qos"`

	// ClientID. Generated if empty.
	ClientID string `json:"clientId"`

	// Username / Password. Either or both can come from secrets instead.
	Username string `json:"username"`
	Password string `json:"password"`

	// CleanSession; default true.
	CleanSession *bool `json:"cleanSession"`
}

func (c *Config) applyDefaults() {
	if c.QoS > 2 {
		c.QoS = 1
	}
	if c.CleanSession == nil {
		t := true
		c.CleanSession = &t
	}
}

type Connector struct{}

func New() sdk.Connector { return &Connector{} }

func (Connector) Manifest() sdk.Manifest {
	return sdk.Manifest{
		ID:          ID,
		Name:        "MQTT",
		Version:     Version,
		Category:    sdk.CategoryIoT,
		Mode:        sdk.ModeStream,
		Description: "Subscribes to an MQTT broker. Each message becomes a record.",
		ConfigSchema: json.RawMessage(`{
			"type": "object",
			"required": ["broker", "topics"],
			"properties": {
				"broker": {"type": "string", "description": "tcp://host:1883 or ssl://host:8883"},
				"topics": {"type": "array", "items": {"type": "string"}, "minItems": 1},
				"qos": {"type": "integer", "minimum": 0, "maximum": 2, "default": 1},
				"clientId": {"type": "string"},
				"username": {"type": "string"},
				"password": {"type": "string"},
				"cleanSession": {"type": "boolean", "default": true}
			}
		}`),
	}
}

func (Connector) Validate(raw json.RawMessage) error {
	if len(raw) == 0 {
		return errors.New("mqtt requires config: broker and topics")
	}
	var c Config
	if err := json.Unmarshal(raw, &c); err != nil {
		return fmt.Errorf("mqtt config: %w", err)
	}
	if c.Broker == "" {
		return errors.New("broker is required")
	}
	if len(c.Topics) == 0 {
		return errors.New("at least one topic is required")
	}
	if c.QoS > 2 {
		return errors.New("qos must be 0, 1, or 2")
	}
	return nil
}

func (Connector) Run(ctx context.Context, rt sdk.Context, raw json.RawMessage) error {
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return err
	}
	cfg.applyDefaults()

	username := cfg.Username
	if username == "" {
		username = rt.Secret("mqtt-username")
	}
	password := cfg.Password
	if password == "" {
		password = rt.Secret("mqtt-password")
	}

	opts := mqttp.NewClientOptions().
		AddBroker(cfg.Broker).
		SetCleanSession(*cfg.CleanSession).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetMaxReconnectInterval(60 * time.Second)

	if cfg.ClientID != "" {
		opts.SetClientID(cfg.ClientID)
	}
	if username != "" {
		opts.SetUsername(username)
		opts.SetPassword(password)
	}

	opts.OnConnect = func(c mqttp.Client) {
		rt.Logger().Info("mqtt connected", "broker", cfg.Broker)
		// Resubscribe on reconnect; paho handles this if CleanSession=false,
		// but we always explicitly re-subscribe so behavior is predictable.
		for _, topic := range cfg.Topics {
			t := c.Subscribe(topic, cfg.QoS, makeMsgHandler(ctx, rt, topic))
			if t.Wait() && t.Error() != nil {
				rt.Logger().Warn("mqtt subscribe failed", "topic", topic, "err", t.Error())
			}
		}
	}
	opts.OnConnectionLost = func(_ mqttp.Client, err error) {
		rt.Logger().Warn("mqtt connection lost", "err", err)
	}

	client := mqttp.NewClient(opts)
	tok := client.Connect()
	// Wait up to 30s for the initial connect; paho will keep retrying after.
	select {
	case <-ctx.Done():
		client.Disconnect(0)
		return ctx.Err()
	case <-tokenChan(tok, 30*time.Second):
		if err := tok.Error(); err != nil {
			return fmt.Errorf("mqtt initial connect: %w", err)
		}
	}

	rt.Logger().Info("mqtt running", "topics", cfg.Topics)

	<-ctx.Done()
	client.Disconnect(250)
	return ctx.Err()
}

// tokenChan turns a paho Token into a channel that closes when the token
// completes or after timeout. We can't directly select on a Token.
func tokenChan(tok mqttp.Token, timeout time.Duration) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		_ = tok.WaitTimeout(timeout)
		close(ch)
	}()
	return ch
}

// counter so we can stamp a per-message correlation ID for dedupe-by-(topic,seq).
var seq atomic.Uint64

func makeMsgHandler(ctx context.Context, rt sdk.Context, _ string) mqttp.MessageHandler {
	return func(_ mqttp.Client, m mqttp.Message) {
		now := time.Now().UTC()
		// Wrap non-JSON payloads.
		var payload json.RawMessage
		if json.Valid(m.Payload()) {
			payload = m.Payload()
		} else {
			b, _ := json.Marshal(map[string]string{"raw": string(m.Payload())})
			payload = b
		}

		tags := map[string]string{
			"topic": m.Topic(),
			"qos":   fmt.Sprintf("%d", m.Qos()),
		}
		if m.Retained() {
			tags["retained"] = "1"
		}

		_ = rt.Publish(ctx, sdk.Record{
			Timestamp: now,
			SourceID:  fmt.Sprintf("%s#%d", m.Topic(), seq.Add(1)),
			Tags:      tags,
			Payload:   payload,
		})
	}
}
