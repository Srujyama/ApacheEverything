package connectors

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"sort"
	"sync"
	"time"

	"github.com/sunny/sunny/apps/server/internal/bus"
	sdk "github.com/sunny/sunny/packages/sdk-go"
)

// InstanceState is the lifecycle state of a running connector instance.
type InstanceState string

const (
	StateStarting InstanceState = "starting"
	StateRunning  InstanceState = "running"
	StateBackoff  InstanceState = "backoff"
	StateStopped  InstanceState = "stopped"
	StateFailed   InstanceState = "failed"
)

// InstanceSpec describes one connector instance to run. Built from config.
type InstanceSpec struct {
	InstanceID string          // unique within the deployment, e.g. "hello-1"
	Type       string          // registered connector ID, e.g. "hello"
	Config     json.RawMessage // raw config blob — connector validates it
}

// InstanceStatus is the externally-visible state of a running instance.
type InstanceStatus struct {
	InstanceID  string        `json:"instanceId"`
	Type        string        `json:"type"`
	State       InstanceState `json:"state"`
	StartedAt   time.Time     `json:"startedAt"`
	Restarts    int           `json:"restarts"`
	LastError   string        `json:"lastError,omitempty"`
	LastErrorAt *time.Time    `json:"lastErrorAt,omitempty"`
}

// Runtime owns the lifecycle of every connector instance.
type Runtime struct {
	bus         *bus.Bus
	logger      *slog.Logger
	secrets     SecretSource
	checkpoints CheckpointStore // may be nil; runtime falls back to in-memory

	mu           sync.RWMutex
	instances    map[string]*instance
	pushHandlers map[string]http.Handler // instanceID → handler; only push connectors populate this
}

type instance struct {
	spec   InstanceSpec
	status InstanceStatus
	cancel context.CancelFunc
	done   chan struct{}
}

// NewRuntime constructs a Runtime. Records produced by instances flow to b.
// Pass a non-nil store to persist connector checkpoints across restarts.
func NewRuntime(b *bus.Bus, logger *slog.Logger, secrets SecretSource, store CheckpointStore) *Runtime {
	if secrets == nil {
		secrets = EnvSecrets{}
	}
	return &Runtime{
		bus:          b,
		logger:       logger,
		secrets:      secrets,
		checkpoints:  store,
		instances:    make(map[string]*instance),
		pushHandlers: make(map[string]http.Handler),
	}
}

// Start launches an instance. Returns an error synchronously if the type isn't
// registered or config validation fails. Once started, the instance runs in a
// supervised goroutine that restarts it on crash with capped exponential backoff.
func (r *Runtime) Start(ctx context.Context, spec InstanceSpec) error {
	connector, ok := Lookup(spec.Type)
	if !ok {
		return fmt.Errorf("connector type %q not registered", spec.Type)
	}
	if err := connector.Validate(spec.Config); err != nil {
		return fmt.Errorf("validate %s: %w", spec.InstanceID, err)
	}

	r.mu.Lock()
	if _, exists := r.instances[spec.InstanceID]; exists {
		r.mu.Unlock()
		return fmt.Errorf("instance %q already running", spec.InstanceID)
	}
	runCtx, cancel := context.WithCancel(ctx)
	inst := &instance{
		spec:   spec,
		cancel: cancel,
		done:   make(chan struct{}),
		status: InstanceStatus{
			InstanceID: spec.InstanceID,
			Type:       spec.Type,
			State:      StateStarting,
			StartedAt:  time.Now().UTC(),
		},
	}
	r.instances[spec.InstanceID] = inst
	r.mu.Unlock()

	// If this connector exposes a push handler, build it now so the HTTP
	// router can mount it. The handler shares the same runtime context as
	// Run for Publish/Logger/Secret semantics.
	if ph, ok := connector.(sdk.PushHandler); ok {
		rtCtx := newRuntimeContext(spec.InstanceID, r.bus, r.logger, r.secrets, r.checkpoints)
		h, err := ph.BuildPushHandler(rtCtx, spec.Config)
		if err != nil {
			cancel()
			r.mu.Lock()
			delete(r.instances, spec.InstanceID)
			r.mu.Unlock()
			return fmt.Errorf("build push handler %s: %w", spec.InstanceID, err)
		}
		r.mu.Lock()
		r.pushHandlers[spec.InstanceID] = h
		r.mu.Unlock()
	}

	go r.supervise(runCtx, inst, connector)
	return nil
}

// PushHandler returns the push handler for an instance, or nil if the
// instance isn't a push connector or doesn't exist.
func (r *Runtime) PushHandler(instanceID string) http.Handler {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.pushHandlers[instanceID]
}

// PushInstances returns the IDs of currently running push connectors.
func (r *Runtime) PushInstances() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.pushHandlers))
	for id := range r.pushHandlers {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func (r *Runtime) supervise(ctx context.Context, inst *instance, c sdk.Connector) {
	defer close(inst.done)

	rtCtx := newRuntimeContext(inst.spec.InstanceID, r.bus, r.logger, r.secrets, r.checkpoints)

	const (
		baseDelay = 500 * time.Millisecond
		maxDelay  = 30 * time.Second
	)
	delay := baseDelay

	for {
		err := r.runOnce(ctx, inst, c, rtCtx)
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			r.setState(inst, StateStopped, nil)
			return
		}
		if err == nil {
			// Connector returned nil — treat as clean exit.
			r.setState(inst, StateStopped, nil)
			return
		}

		r.setState(inst, StateBackoff, err)
		select {
		case <-ctx.Done():
			r.setState(inst, StateStopped, nil)
			return
		case <-time.After(delay):
		}

		r.mu.Lock()
		inst.status.Restarts++
		r.mu.Unlock()
		if delay < maxDelay {
			delay *= 2
			if delay > maxDelay {
				delay = maxDelay
			}
		}
	}
}

// runOnce runs the connector once, recovering from panics so the supervisor
// can apply backoff instead of crashing the whole server.
func (r *Runtime) runOnce(ctx context.Context, inst *instance, c sdk.Connector, rtCtx sdk.Context) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			r.logger.Error("connector panic",
				"instance", inst.spec.InstanceID,
				"panic", rec,
				"stack", string(debug.Stack()))
			err = fmt.Errorf("panic: %v", rec)
		}
	}()
	r.setState(inst, StateRunning, nil)
	r.logger.Info("connector started", "instance", inst.spec.InstanceID, "type", inst.spec.Type)
	return c.Run(ctx, rtCtx, inst.spec.Config)
}

func (r *Runtime) setState(inst *instance, state InstanceState, err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	inst.status.State = state
	if err != nil {
		now := time.Now().UTC()
		inst.status.LastError = err.Error()
		inst.status.LastErrorAt = &now
	}
}

// Stop signals an instance to stop and waits up to timeout for it to exit.
func (r *Runtime) Stop(instanceID string, timeout time.Duration) error {
	r.mu.Lock()
	inst, ok := r.instances[instanceID]
	r.mu.Unlock()
	if !ok {
		return fmt.Errorf("instance %q not running", instanceID)
	}
	inst.cancel()
	select {
	case <-inst.done:
	case <-time.After(timeout):
		return fmt.Errorf("instance %q did not stop within %s", instanceID, timeout)
	}
	r.mu.Lock()
	delete(r.instances, instanceID)
	r.mu.Unlock()
	return nil
}

// StopAll stops every instance, waiting up to timeout per instance.
func (r *Runtime) StopAll(timeout time.Duration) {
	r.mu.RLock()
	ids := make([]string, 0, len(r.instances))
	for id := range r.instances {
		ids = append(ids, id)
	}
	r.mu.RUnlock()

	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if err := r.Stop(id, timeout); err != nil {
				r.logger.Warn("stop instance", "instance", id, "err", err)
			}
		}(id)
	}
	wg.Wait()
}

// Statuses returns a snapshot of all running instance statuses, sorted by ID.
func (r *Runtime) Statuses() []InstanceStatus {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]InstanceStatus, 0, len(r.instances))
	for _, inst := range r.instances {
		out = append(out, inst.status)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].InstanceID < out[j].InstanceID })
	return out
}

// Status returns the status of a single instance.
func (r *Runtime) Status(instanceID string) (InstanceStatus, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	inst, ok := r.instances[instanceID]
	if !ok {
		return InstanceStatus{}, false
	}
	return inst.status, true
}
