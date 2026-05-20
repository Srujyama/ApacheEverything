// doctor.go implements the `sunny-cli doctor` subcommand.
//
// Runs a series of read-only checks against a Sunny server and prints PASS/FAIL
// per check. Designed for first-install troubleshooting and for CI smoke tests
// after a deploy. Never modifies state.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

func doctorCmd(args []string) error {
	server := os.Getenv("SUNNY_SERVER")
	if server == "" {
		server = "http://localhost:3000"
	}
	token := os.Getenv("SUNNY_TOKEN")
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
		default:
			return fmt.Errorf("unexpected arg: %s", args[i])
		}
	}
	server = strings.TrimRight(server, "/")

	type check struct {
		name string
		fn   func(context.Context) error
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	hc := &http.Client{Timeout: 10 * time.Second}
	get := func(ctx context.Context, path string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, server+path, nil)
		if err != nil {
			return nil, err
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		return hc.Do(req)
	}

	checks := []check{
		{"server reachable", func(ctx context.Context) error {
			res, err := get(ctx, "/api/health")
			if err != nil {
				return err
			}
			defer res.Body.Close()
			if res.StatusCode != 200 {
				return fmt.Errorf("HTTP %d", res.StatusCode)
			}
			return nil
		}},
		{"liveness probe", func(ctx context.Context) error {
			res, err := get(ctx, "/healthz")
			if err != nil {
				return err
			}
			defer res.Body.Close()
			if res.StatusCode != 200 {
				return fmt.Errorf("HTTP %d", res.StatusCode)
			}
			return nil
		}},
		{"readiness probe", func(ctx context.Context) error {
			res, err := get(ctx, "/readyz")
			if err != nil {
				return err
			}
			defer res.Body.Close()
			if res.StatusCode != 200 {
				b, _ := io.ReadAll(res.Body)
				return fmt.Errorf("HTTP %d: %s", res.StatusCode, strings.TrimSpace(string(b)))
			}
			return nil
		}},
		{"version endpoint reports apiVersion", func(ctx context.Context) error {
			res, err := get(ctx, "/api/version")
			if err != nil {
				return err
			}
			defer res.Body.Close()
			var v map[string]any
			if err := json.NewDecoder(res.Body).Decode(&v); err != nil {
				return err
			}
			if _, ok := v["apiVersion"]; !ok {
				return errors.New("missing apiVersion in /api/version response")
			}
			return nil
		}},
		{"versioned /api/v1/version works", func(ctx context.Context) error {
			res, err := get(ctx, "/api/v1/version")
			if err != nil {
				return err
			}
			defer res.Body.Close()
			if res.StatusCode != 200 {
				return fmt.Errorf("HTTP %d", res.StatusCode)
			}
			return nil
		}},
		{"prometheus /metrics endpoint", func(ctx context.Context) error {
			res, err := get(ctx, "/metrics")
			if err != nil {
				return err
			}
			defer res.Body.Close()
			if res.StatusCode != 200 {
				return fmt.Errorf("HTTP %d", res.StatusCode)
			}
			body, _ := io.ReadAll(res.Body)
			if !strings.Contains(string(body), "sunny_build_info") {
				return errors.New("metrics response missing sunny_build_info")
			}
			return nil
		}},
		{"auth status reachable", func(ctx context.Context) error {
			res, err := get(ctx, "/api/auth/status")
			if err != nil {
				return err
			}
			defer res.Body.Close()
			if res.StatusCode != 200 {
				return fmt.Errorf("HTTP %d", res.StatusCode)
			}
			return nil
		}},
		{"connectors list (auth-gated)", func(ctx context.Context) error {
			res, err := get(ctx, "/api/connectors/instances")
			if err != nil {
				return err
			}
			defer res.Body.Close()
			if res.StatusCode == 401 {
				return errors.New("server requires auth; set SUNNY_TOKEN or --token")
			}
			if res.StatusCode != 200 {
				return fmt.Errorf("HTTP %d", res.StatusCode)
			}
			return nil
		}},
	}

	pass, fail := 0, 0
	for _, c := range checks {
		err := c.fn(ctx)
		if err != nil {
			fmt.Fprintf(os.Stdout, "  FAIL  %s — %v\n", c.name, err)
			fail++
		} else {
			fmt.Fprintf(os.Stdout, "  PASS  %s\n", c.name)
			pass++
		}
	}
	fmt.Fprintf(os.Stdout, "\n%d passed, %d failed.\n", pass, fail)
	if fail > 0 {
		return fmt.Errorf("%d check(s) failed", fail)
	}
	return nil
}
