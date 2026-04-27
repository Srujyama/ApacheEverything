package connectors

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"

	"github.com/sunny/sunny/apps/server/internal/bus"
	sdk "github.com/sunny/sunny/packages/sdk-go"
)

// CheckpointStore persists per-instance resume values.
//
// Implementations live outside this package (storage.Storage satisfies it)
// to avoid an import cycle. If a Runtime is constructed with a nil store,
// it falls back to an in-memory map — useful for tests.
type CheckpointStore interface {
	SaveCheckpoint(ctx context.Context, instanceID, key, value string) error
	LoadCheckpoint(ctx context.Context, instanceID, key string) (string, error)
}

// runtimeContext is the per-instance sdk.Context implementation. One of these
// is constructed by Runtime.Start for each connector instance.
type runtimeContext struct {
	instanceID string
	bus        *bus.Bus
	logger     *slogLogger
	secrets    SecretSource
	store      CheckpointStore // may be nil; falls back to in-memory

	mu          sync.Mutex
	checkpoints map[string]string // in-memory fallback
}

// SecretSource resolves a connector secret by name. Phase 1 implementation
// reads from environment variables; phase 4 swaps in a vault-backed source.
type SecretSource interface {
	Secret(name string) string
}

// EnvSecrets reads secrets from SUNNY_SECRET_<UPPER_NAME>.
type EnvSecrets struct{}

func (EnvSecrets) Secret(name string) string {
	key := "SUNNY_SECRET_" + strings.ToUpper(strings.ReplaceAll(name, "-", "_"))
	return os.Getenv(key)
}

func newRuntimeContext(instanceID string, b *bus.Bus, baseLogger *slog.Logger, secrets SecretSource, store CheckpointStore) *runtimeContext {
	return &runtimeContext{
		instanceID:  instanceID,
		bus:         b,
		logger:      &slogLogger{l: baseLogger.With("connector_instance", instanceID)},
		secrets:     secrets,
		store:       store,
		checkpoints: make(map[string]string),
	}
}

// Publish stamps the record with the instance ID (if the connector didn't set
// one) and hands it to the bus.
func (c *runtimeContext) Publish(ctx context.Context, r sdk.Record) error {
	if r.ConnectorID == "" {
		r.ConnectorID = c.instanceID
	}
	c.bus.Publish(ctx, r)
	return nil
}

func (c *runtimeContext) Logger() sdk.Logger {
	return c.logger
}

func (c *runtimeContext) Secret(name string) string {
	if c.secrets == nil {
		return ""
	}
	return c.secrets.Secret(name)
}

func (c *runtimeContext) Checkpoint(ctx context.Context, key, value string) error {
	if c.store != nil {
		return c.store.SaveCheckpoint(ctx, c.instanceID, key, value)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checkpoints[key] = value
	return nil
}

func (c *runtimeContext) LoadCheckpoint(ctx context.Context, key string) (string, error) {
	if c.store != nil {
		return c.store.LoadCheckpoint(ctx, c.instanceID, key)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.checkpoints[key], nil
}

// slogLogger adapts *slog.Logger to sdk.Logger.
type slogLogger struct{ l *slog.Logger }

func (s *slogLogger) Debug(msg string, args ...any) { s.l.Debug(msg, args...) }
func (s *slogLogger) Info(msg string, args ...any)  { s.l.Info(msg, args...) }
func (s *slogLogger) Warn(msg string, args ...any)  { s.l.Warn(msg, args...) }
func (s *slogLogger) Error(msg string, args ...any) { s.l.Error(msg, args...) }
