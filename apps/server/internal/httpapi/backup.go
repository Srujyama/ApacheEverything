package httpapi

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// backupAPI streams a gzipped tar of the server's data directory.
//
//	POST /api/backup
//
// Auth-protected (heavy: reads the whole DuckDB file). Returns
// application/gzip with Content-Disposition. Skips the .duckdb.wal file.
//
// Like the offline `sunny-cli backup`, this is a best-effort snapshot —
// DuckDB writes during the read can produce a slightly inconsistent file.
// For a strictly consistent backup, use the offline path or DuckDB's
// EXPORT DATABASE (a future enhancement).
type backupAPI struct {
	dataDir string
}

func (b *backupAPI) handle(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "use GET or POST", http.StatusMethodNotAllowed)
		return
	}
	info, err := os.Stat(b.dataDir)
	if err != nil {
		http.Error(w, "data dir not readable: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if !info.IsDir() {
		http.Error(w, "data dir is not a directory", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", `attachment; filename="sunny-backup.tar.gz"`)

	gz := gzip.NewWriter(w)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	_ = filepath.Walk(b.dataDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if strings.HasSuffix(path, ".duckdb.wal") {
			return nil
		}
		rel, err := filepath.Rel(b.dataDir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = rel
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, _ = io.Copy(tw, f)
		return nil
	})
}
