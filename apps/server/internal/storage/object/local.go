// local.go is the filesystem-backed ObjectStore.
//
// Useful for:
//
//   - Laptop installs (no cloud account required).
//   - CI / tests (deterministic, no Docker required).
//   - Air-gapped deployments where the "object store" is a shared NFS mount.
//
// Keys map directly to paths under root. Subdirectories are created on
// demand. Atomic writes use the tmpfile-then-rename pattern so concurrent
// readers never see a partial object.

package object

import (
	"context"
	"errors"
	"fmt"
	"io"
	"iter"
	"os"
	"path/filepath"
	"strings"
)

// LocalObjectStore is the filesystem-backed ObjectStore.
type LocalObjectStore struct {
	root string
}

// NewLocalObjectStore returns a store rooted at the given directory. The
// directory is created if it doesn't exist.
func NewLocalObjectStore(root string) (*LocalObjectStore, error) {
	if root == "" {
		return nil, errors.New("local: empty root")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("local: mkdir %s: %w", abs, err)
	}
	return &LocalObjectStore{root: abs}, nil
}

func (l *LocalObjectStore) Scheme() string { return "file" }

func (l *LocalObjectStore) resolve(key string) (string, error) {
	// Reject path traversal. After Clean, the path must not start with ".."
	// and must stay inside l.root.
	cleaned := filepath.Clean("/" + key)
	abs := filepath.Join(l.root, cleaned)
	if !strings.HasPrefix(abs, l.root+string(os.PathSeparator)) && abs != l.root {
		return "", fmt.Errorf("local: key escapes root: %q", key)
	}
	return abs, nil
}

func (l *LocalObjectStore) Get(_ context.Context, key string) (io.ReadCloser, error) {
	path, err := l.resolve(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	return f, err
}

func (l *LocalObjectStore) Put(_ context.Context, key string, body io.Reader) error {
	path, err := l.resolve(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	// Write to tmpfile, fsync, rename. Renames within a filesystem are
	// atomic on POSIX so concurrent readers either see the old object
	// or the new one — never a partial.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".obj-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	defer func() {
		// In the happy path we already renamed; remove is then a no-op.
		cleanup()
	}()
	if _, err := io.Copy(tmp, body); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	// Disable cleanup: the rename consumed the tmp file.
	cleanup = func() {}
	return nil
}

func (l *LocalObjectStore) Delete(_ context.Context, key string) error {
	path, err := l.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			// Match S3 / GCS idempotent delete.
			return nil
		}
		return err
	}
	return nil
}

func (l *LocalObjectStore) Stat(_ context.Context, key string) (ObjectInfo, error) {
	path, err := l.resolve(key)
	if err != nil {
		return ObjectInfo{}, err
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return ObjectInfo{}, fmt.Errorf("%w: %s", ErrNotFound, key)
	}
	if err != nil {
		return ObjectInfo{}, err
	}
	return ObjectInfo{Key: key, Size: info.Size(), ModTime: info.ModTime()}, nil
}

func (l *LocalObjectStore) List(ctx context.Context, prefix string) iter.Seq2[ObjectInfo, error] {
	return func(yield func(ObjectInfo, error) bool) {
		startDir, err := l.resolve(prefix)
		if err != nil {
			yield(ObjectInfo{}, err)
			return
		}
		// If prefix isn't a directory yet, walk the parent and filter.
		walkRoot := startDir
		if info, err := os.Stat(startDir); err == nil && !info.IsDir() {
			// Single-file prefix.
			rel, _ := filepath.Rel(l.root, startDir)
			yield(ObjectInfo{Key: filepath.ToSlash(rel), Size: info.Size(), ModTime: info.ModTime()}, nil)
			return
		} else if errors.Is(err, os.ErrNotExist) {
			// Walk the parent with the leaf as a filter.
			walkRoot = filepath.Dir(startDir)
		}
		filterPrefix := strings.ToLower(filepath.Base(startDir))
		walkInPrefix := filterPrefix != ""

		err = filepath.Walk(walkRoot, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				if errors.Is(walkErr, os.ErrNotExist) {
					return nil
				}
				return walkErr
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if info.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(l.root, path)
			if err != nil {
				return err
			}
			key := filepath.ToSlash(rel)
			if walkInPrefix && !strings.HasPrefix(filepath.Base(path), filterPrefix) && filterPrefix != strings.ToLower(filepath.Base(path)) {
				// We're filtering within the parent dir, the file must
				// either start with the filter or be inside a subtree
				// whose root matches. The relpath check below is the
				// canonical filter.
			}
			// Canonical filter: rel path must start with the user-supplied prefix.
			if prefix != "" && !strings.HasPrefix(key, strings.TrimPrefix(prefix, "/")) {
				return nil
			}
			if !yield(ObjectInfo{Key: key, Size: info.Size(), ModTime: info.ModTime()}, nil) {
				return errStopWalk
			}
			return nil
		})
		if err != nil && !errors.Is(err, errStopWalk) {
			yield(ObjectInfo{}, err)
		}
	}
}

// errStopWalk is a sentinel used to bail out of filepath.Walk when the
// iterator consumer stops pulling.
var errStopWalk = errors.New("stop walk")
