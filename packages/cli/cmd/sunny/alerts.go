// alerts.go implements `sunny-cli alerts <subcommand>`.
//
//   sunny-cli alerts deadletters [--server URL] [--token T] [--limit N]
//
// Lists alerts whose notifier delivery exhausted retries. Useful when a
// Slack webhook expired or a downstream receiver went offline.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
)

func alertsCmd(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: sunny-cli alerts <deadletters> [flags]")
	}
	sub := args[0]
	rest := args[1:]
	switch sub {
	case "deadletters", "dlq":
		return alertsDeadLettersCmd(rest)
	default:
		return fmt.Errorf("unknown alerts subcommand: %s", sub)
	}
}

func alertsDeadLettersCmd(args []string) error {
	server := os.Getenv("SUNNY_SERVER")
	if server == "" {
		server = "http://localhost:3000"
	}
	token := os.Getenv("SUNNY_TOKEN")
	limit := 50
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
		case "--limit":
			if i+1 >= len(args) {
				return errors.New("--limit needs a value")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil {
				return fmt.Errorf("--limit: %w", err)
			}
			limit = n; i++
		default:
			return fmt.Errorf("unexpected arg: %s", args[i])
		}
	}
	url := fmt.Sprintf("%s/api/alerts/deadletters?limit=%d",
		strings.TrimRight(server, "/"), limit)
	req, err := http.NewRequest(http.MethodGet, url, nil)
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
	if res.StatusCode == http.StatusServiceUnavailable {
		return errors.New("server has no dead-letter store configured (set SUNNY_ALERTS_DLQ_PATH)")
	}
	if res.StatusCode != 200 {
		b, _ := io.ReadAll(res.Body)
		return fmt.Errorf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
	}
	var dls []struct {
		AlertID   string `json:"alertId"`
		Notifier  string `json:"notifier"`
		Reason    string `json:"reason"`
		Attempts  int    `json:"attempts"`
		LastTried string `json:"lastTried"`
	}
	if err := json.NewDecoder(res.Body).Decode(&dls); err != nil {
		return err
	}
	if len(dls) == 0 {
		fmt.Fprintln(os.Stderr, "no dead letters")
		return nil
	}
	rows := make([][]any, len(dls))
	for i, dl := range dls {
		rows[i] = []any{dl.AlertID, dl.Notifier, dl.Attempts, fmtSince(dl.LastTried), trunc(dl.Reason, 60)}
	}
	renderTable(os.Stdout, []string{"ALERT", "NOTIFIER", "ATTEMPTS", "LAST", "REASON"}, rows)
	fmt.Fprintf(os.Stderr, "(%d entries)\n", len(dls))
	return nil
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
