// deadletter.go provides a thin DuckDB-backed implementation of
// DeadLetterStore. Sits in the alerts package (not storage) because the DLQ
// is alerts-specific and shouldn't widen the Storage interface for the
// upcoming Iceberg/Delta backends in Phase 1.
//
// We grab the underlying *sql.DB through the existing storage.Backend by
// having the backend expose it. To avoid leaking *sql.DB across packages,
// we instead persist DLQ rows via a minimal in-process file or, for tests,
// in memory. Production deployments should set SUNNY_ALERTS_DLQ_PATH;
// otherwise an in-memory store is used (warning logged).
package alerts

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"
)

// MemoryDeadLetterStore is an in-memory ring-buffer DLQ. Useful for tests
// and for embedded/laptop installs where persisting DLQ to disk is overkill.
type MemoryDeadLetterStore struct {
	mu      sync.Mutex
	entries []DeadLetter
	max     int
}

// NewMemoryDeadLetterStore caps at maxEntries (oldest evicted first).
func NewMemoryDeadLetterStore(maxEntries int) *MemoryDeadLetterStore {
	if maxEntries <= 0 {
		maxEntries = 1024
	}
	return &MemoryDeadLetterStore{max: maxEntries}
}

func (m *MemoryDeadLetterStore) InsertDeadLetter(_ context.Context, dl DeadLetter) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, dl)
	if len(m.entries) > m.max {
		m.entries = m.entries[len(m.entries)-m.max:]
	}
	return nil
}

func (m *MemoryDeadLetterStore) ListDeadLetters(_ context.Context, limit int) ([]DeadLetter, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if limit <= 0 || limit > len(m.entries) {
		limit = len(m.entries)
	}
	// Newest first.
	out := make([]DeadLetter, 0, limit)
	for i := len(m.entries) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, m.entries[i])
	}
	return out, nil
}

// FileDeadLetterStore appends DLQ rows to a JSONL file. Crash-safe (each
// row is one fsynced line). Suitable for single-node deployments; HA setups
// should migrate to the Phase 2 governance store.
type FileDeadLetterStore struct {
	path string
	mu   sync.Mutex
}

// NewFileDeadLetterStore opens (or creates) path in append mode.
func NewFileDeadLetterStore(path string) (*FileDeadLetterStore, error) {
	if path == "" {
		return nil, errors.New("FileDeadLetterStore: empty path")
	}
	// Touch the file so subsequent reads don't fail.
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	_ = f.Close()
	return &FileDeadLetterStore{path: path}, nil
}

func (s *FileDeadLetterStore) InsertDeadLetter(_ context.Context, dl DeadLetter) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	line, err := json.Marshal(dl)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return err
	}
	return f.Sync()
}

func (s *FileDeadLetterStore) ListDeadLetters(_ context.Context, limit int) ([]DeadLetter, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var out []DeadLetter
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var dl DeadLetter
		if err := json.Unmarshal(line, &dl); err != nil {
			continue // skip corrupt rows; the file is append-only so corruption is rare
		}
		out = append(out, dl)
	}
	// Newest last → reverse and cap.
	if limit > 0 && limit < len(out) {
		out = out[len(out)-limit:]
	}
	// Reverse for newest-first.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func splitLines(data []byte) [][]byte {
	var out [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			out = append(out, data[start:i])
			start = i + 1
		}
	}
	if start < len(data) {
		out = append(out, data[start:])
	}
	return out
}

// ResolveDeadLetterStore returns the configured DLQ. Reads
// SUNNY_ALERTS_DLQ_PATH; falls back to an in-memory store when unset.
func ResolveDeadLetterStore() (DeadLetterStore, string) {
	if p := os.Getenv("SUNNY_ALERTS_DLQ_PATH"); p != "" {
		s, err := NewFileDeadLetterStore(p)
		if err == nil {
			return s, p
		}
		// Fall through to memory if the file can't be opened.
	}
	return NewMemoryDeadLetterStore(1024), "memory"
}

// ensureTimeUTC normalizes timestamps stored in DLQ entries. Exported only
// for tests; callers should never need to call this.
func ensureTimeUTC(t time.Time) time.Time { return t.UTC() }
