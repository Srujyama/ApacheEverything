// Package main is the `sunny-cli` admin tool.
//
// Subcommands:
//   - version            print the CLI version
//   - hash-password      bcrypt-hash a password for SUNNY_PASSWORD_HASH
//   - backup <data> <out.tar.gz>     snapshot the data directory
//   - restore <in.tar.gz> <data>     restore a snapshot into the data directory
//   - query <SQL>                    run a SQL query against /api/query
//
// The CLI is intentionally small. The main `sunny` server binary handles
// running the platform.
package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/crypto/bcrypt"
)

const Version = "0.1.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(0)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "version", "--version", "-v":
		fmt.Println(Version)
	case "help", "--help", "-h":
		usage()
	case "hash-password":
		err = hashPasswordCmd(args)
	case "backup":
		err = backupCmd(args)
	case "restore":
		err = restoreCmd(args)
	case "query":
		err = queryCmd(args)
	case "watch":
		err = watchCmd(args)
	case "connectors":
		err = connectorsCmd(args)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", cmd)
		usage()
		os.Exit(1)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Println("sunny-cli — admin helper for Sunny")
	fmt.Println()
	fmt.Println("usage:")
	fmt.Println("  sunny-cli version")
	fmt.Println("  sunny-cli hash-password <password>")
	fmt.Println("  sunny-cli backup  <data-dir> <out.tar.gz>")
	fmt.Println("  sunny-cli restore <in.tar.gz> <data-dir>")
	fmt.Println("  sunny-cli query   [--server URL] [--token T] \"SELECT ...\"")
	fmt.Println("  sunny-cli watch   [--server URL] [--token T] [--connector ID]")
	fmt.Println("  sunny-cli connectors [instances|types]  list running or available connectors")
	fmt.Println()
	fmt.Println("Server URL defaults to http://localhost:3000 (or $SUNNY_SERVER).")
	fmt.Println("Auth token defaults to $SUNNY_TOKEN.")
	fmt.Println()
	fmt.Println("To run the server, use the main `sunny` binary.")
}

func hashPasswordCmd(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: sunny-cli hash-password <password>")
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(args[0]), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	fmt.Println(string(hash))
	fmt.Fprintln(os.Stderr, "set as: export SUNNY_PASSWORD_HASH='"+string(hash)+"'")
	return nil
}

// backupCmd creates a gzipped tar of the data directory. DuckDB files written
// while we're running may be in an inconsistent state — this is a best-effort
// snapshot suitable for "stopped server" backups. Online consistency would
// need a proper DuckDB EXPORT DATABASE call; that's a phase-7 enhancement.
func backupCmd(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: sunny-cli backup <data-dir> <out.tar.gz>")
	}
	srcDir := args[0]
	outPath := args[1]

	info, err := os.Stat(srcDir)
	if err != nil {
		return fmt.Errorf("data dir: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", srcDir)
	}

	out, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer out.Close()

	gz := gzip.NewWriter(out)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	count := 0
	var bytesCopied int64
	err = filepath.Walk(srcDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		// Skip the WAL — DuckDB regenerates it on next start.
		if strings.HasSuffix(path, ".duckdb.wal") {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
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
		n, err := io.Copy(tw, f)
		bytesCopied += n
		count++
		return err
	})
	if err != nil {
		return err
	}
	fmt.Printf("Wrote %s (%d files, %s)\n", outPath, count, humanBytes(bytesCopied))
	fmt.Println("Stop the sunny server before backing up for a strictly consistent snapshot.")
	return nil
}

func restoreCmd(args []string) error {
	if len(args) != 2 {
		return errors.New("usage: sunny-cli restore <in.tar.gz> <data-dir>")
	}
	inPath := args[0]
	dstDir := args[1]

	if entries, err := os.ReadDir(dstDir); err == nil && len(entries) > 0 {
		return fmt.Errorf("destination %s is not empty; refusing to overwrite", dstDir)
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}

	in, err := os.Open(inPath)
	if err != nil {
		return err
	}
	defer in.Close()
	gz, err := gzip.NewReader(in)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	count := 0
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// Reject path traversal.
		if strings.Contains(hdr.Name, "..") || strings.HasPrefix(hdr.Name, "/") {
			return fmt.Errorf("refusing unsafe entry: %s", hdr.Name)
		}
		dst := filepath.Join(dstDir, hdr.Name)
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(dst, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}
			f.Close()
			_ = os.Chtimes(dst, time.Now(), hdr.ModTime)
			count++
		}
	}
	fmt.Printf("Restored %d files into %s\n", count, dstDir)
	return nil
}

func humanBytes(n int64) string {
	const k = 1024
	if n < k {
		return fmt.Sprintf("%d B", n)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	v := float64(n)
	u := -1
	for v >= k && u < len(units)-1 {
		v /= k
		u++
	}
	return fmt.Sprintf("%.1f %s", v, units[u])
}

// queryCmd POSTs to /api/query and renders the result as a table.
//
// Args:
//   sunny-cli query [--server URL] [--token T] [--limit N] "SELECT ..."
func queryCmd(args []string) error {
	server := os.Getenv("SUNNY_SERVER")
	if server == "" {
		server = "http://localhost:3000"
	}
	token := os.Getenv("SUNNY_TOKEN")
	limit := 0

	var sql string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--server":
			if i+1 >= len(args) {
				return errors.New("--server needs a value")
			}
			server = args[i+1]
			i++
		case "--token":
			if i+1 >= len(args) {
				return errors.New("--token needs a value")
			}
			token = args[i+1]
			i++
		case "--limit":
			if i+1 >= len(args) {
				return errors.New("--limit needs a value")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil {
				return fmt.Errorf("--limit: %w", err)
			}
			limit = n
			i++
		default:
			if sql != "" {
				return errors.New("usage: sunny-cli query [--server URL] [--token T] \"SELECT ...\"")
			}
			sql = args[i]
		}
	}
	if sql == "" {
		return errors.New("no SQL provided")
	}

	body, _ := json.Marshal(map[string]any{
		"sql":   sql,
		"limit": limit,
	})
	req, err := http.NewRequest("POST", strings.TrimRight(server, "/")+"/api/query", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}

	var result struct {
		Columns  []string `json:"columns"`
		Rows     [][]any  `json:"rows"`
		RowCount int64    `json:"rowCount"`
	}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return err
	}
	renderTable(os.Stdout, result.Columns, result.Rows)
	fmt.Fprintf(os.Stderr, "(%d rows)\n", result.RowCount)
	return nil
}

// renderTable writes a simple ASCII table. Wide enough for casual use,
// not trying to match psql's polish.
func renderTable(w io.Writer, cols []string, rows [][]any) {
	widths := make([]int, len(cols))
	for i, c := range cols {
		widths[i] = len(c)
	}
	cells := make([][]string, len(rows))
	for r, row := range rows {
		cells[r] = make([]string, len(cols))
		for c, v := range row {
			s := fmtCell(v)
			cells[r][c] = s
			if len(s) > widths[c] {
				widths[c] = len(s)
			}
		}
	}

	// header
	for i, c := range cols {
		if i > 0 {
			fmt.Fprint(w, " | ")
		}
		fmt.Fprintf(w, "%-*s", widths[i], c)
	}
	fmt.Fprintln(w)
	// separator
	for i := range cols {
		if i > 0 {
			fmt.Fprint(w, "-+-")
		}
		fmt.Fprint(w, strings.Repeat("-", widths[i]))
	}
	fmt.Fprintln(w)
	// rows
	for _, row := range cells {
		for i, v := range row {
			if i > 0 {
				fmt.Fprint(w, " | ")
			}
			fmt.Fprintf(w, "%-*s", widths[i], v)
		}
		fmt.Fprintln(w)
	}
}

func fmtCell(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case float64:
		// JSON numbers come back as float64. Render integer-valued
		// floats without a trailing ".0" so columns line up nicely.
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'g', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

// watchCmd opens the /api/stream WebSocket and prints records as they
// arrive. Defaults: --server $SUNNY_SERVER (or localhost:3000),
// --token $SUNNY_TOKEN. Optional --connector filters server-side.
func watchCmd(args []string) error {
	server := os.Getenv("SUNNY_SERVER")
	if server == "" {
		server = "http://localhost:3000"
	}
	token := os.Getenv("SUNNY_TOKEN")
	connector := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--server":
			if i+1 >= len(args) {
				return errors.New("--server needs a value")
			}
			server = args[i+1]; i++
		case "--token":
			if i+1 >= len(args) {
				return errors.New("--token needs a value")
			}
			token = args[i+1]; i++
		case "--connector":
			if i+1 >= len(args) {
				return errors.New("--connector needs a value")
			}
			connector = args[i+1]; i++
		default:
			return fmt.Errorf("unexpected arg: %s", args[i])
		}
	}

	u := strings.TrimRight(server, "/") + "/api/stream"
	if connector != "" {
		u += "?connector=" + connector
	}
	// http→ws / https→wss.
	if strings.HasPrefix(u, "https://") {
		u = "wss://" + u[len("https://"):]
	} else if strings.HasPrefix(u, "http://") {
		u = "ws://" + u[len("http://"):]
	}

	headers := http.Header{}
	if token != "" {
		headers.Set("Authorization", "Bearer "+token)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn, _, err := websocket.Dial(ctx, u, &websocket.DialOptions{
		HTTPHeader: headers,
	})
	if err != nil {
		return fmt.Errorf("dial %s: %w", u, err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	fmt.Fprintln(os.Stderr, "watching", u, "(Ctrl-C to stop)")

	// Handle SIGINT cleanly.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	for {
		_, data, err := conn.Read(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			return err
		}
		var rec struct {
			Timestamp   string         `json:"timestamp"`
			ConnectorID string         `json:"connectorId"`
			SourceID    string         `json:"sourceId"`
			Tags        map[string]any `json:"tags"`
			Payload     map[string]any `json:"payload"`
		}
		if err := json.Unmarshal(data, &rec); err != nil {
			continue
		}
		// Compose a one-line summary.
		head := pickHeadline(rec.Payload)
		t := rec.Timestamp
		if len(t) > 19 {
			t = t[:19] + "Z"
		}
		fmt.Printf("%s  %-22s  %s\n", t, rec.ConnectorID, head)
	}
}

func pickHeadline(payload map[string]any) string {
	for _, k := range []string{"headline", "event", "place", "siteName", "parameter", "greeting"} {
		if v, ok := payload[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				return s
			}
		}
	}
	if len(payload) == 0 {
		return ""
	}
	b, _ := json.Marshal(payload)
	if len(b) > 80 {
		return string(b[:80]) + "…"
	}
	return string(b)
}

// connectorsCmd lists running instances or registered types.
//
//   sunny-cli connectors                # running instances (default)
//   sunny-cli connectors types          # all registered types
func connectorsCmd(args []string) error {
	server := os.Getenv("SUNNY_SERVER")
	if server == "" {
		server = "http://localhost:3000"
	}
	token := os.Getenv("SUNNY_TOKEN")
	mode := "instances"

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--server":
			if i+1 >= len(args) {
				return errors.New("--server needs a value")
			}
			server = args[i+1]; i++
		case "--token":
			if i+1 >= len(args) {
				return errors.New("--token needs a value")
			}
			token = args[i+1]; i++
		case "instances", "types":
			mode = args[i]
		default:
			return fmt.Errorf("unexpected arg: %s", args[i])
		}
	}

	path := "/api/connectors/instances"
	if mode == "types" {
		path = "/api/connectors"
	}
	req, err := http.NewRequest("GET", strings.TrimRight(server, "/")+path, nil)
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != 200 {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}

	if mode == "instances" {
		var insts []struct {
			InstanceID string `json:"instanceId"`
			Type       string `json:"type"`
			State      string `json:"state"`
			Restarts   int    `json:"restarts"`
			StartedAt  string `json:"startedAt"`
			LastError  string `json:"lastError"`
		}
		if err := json.NewDecoder(res.Body).Decode(&insts); err != nil {
			return err
		}
		if len(insts) == 0 {
			fmt.Fprintln(os.Stderr, "no running instances")
			return nil
		}
		rows := make([][]any, len(insts))
		for i, inst := range insts {
			rows[i] = []any{inst.InstanceID, inst.Type, inst.State, inst.Restarts, fmtSince(inst.StartedAt)}
		}
		renderTable(os.Stdout, []string{"INSTANCE", "TYPE", "STATE", "RESTARTS", "AGE"}, rows)
		return nil
	}

	// types
	var resp struct {
		Types []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			Mode     string `json:"mode"`
			Category string `json:"category"`
			Version  string `json:"version"`
		} `json:"types"`
	}
	if err := json.NewDecoder(res.Body).Decode(&resp); err != nil {
		return err
	}
	rows := make([][]any, len(resp.Types))
	for i, t := range resp.Types {
		rows[i] = []any{t.ID, t.Name, t.Mode, t.Category, t.Version}
	}
	renderTable(os.Stdout, []string{"ID", "NAME", "MODE", "CATEGORY", "VERSION"}, rows)
	return nil
}

func fmtSince(rfc3339 string) string {
	t, err := time.Parse(time.RFC3339Nano, rfc3339)
	if err != nil {
		t, err = time.Parse(time.RFC3339, rfc3339)
		if err != nil {
			return rfc3339
		}
	}
	d := time.Since(t).Round(time.Second)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
