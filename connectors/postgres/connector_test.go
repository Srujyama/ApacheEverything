package postgres

import (
	"context"
	"encoding/json"
	"sync"
	"testing"

	sdk "github.com/sunny/sunny/packages/sdk-go"
)

type fakeCtx struct {
	mu      sync.Mutex
	secrets map[string]string
}

func (f *fakeCtx) Publish(_ context.Context, _ sdk.Record) error                      { return nil }
func (f *fakeCtx) Logger() sdk.Logger                                                 { return noopLogger{} }
func (f *fakeCtx) Secret(name string) string                                          { return f.secrets[name] }
func (f *fakeCtx) Checkpoint(_ context.Context, _ string, _ string) error             { return nil }
func (f *fakeCtx) LoadCheckpoint(_ context.Context, _ string) (string, error)         { return "", nil }

type noopLogger struct{}

func (noopLogger) Debug(string, ...any) {}
func (noopLogger) Info(string, ...any)  {}
func (noopLogger) Warn(string, ...any)  {}
func (noopLogger) Error(string, ...any) {}

func TestManifest(t *testing.T) {
	m := New().Manifest()
	if m.ID != ID || m.Mode != "stream" {
		t.Fatalf("manifest mismatch: %+v", m)
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		body string
		ok   bool
	}{
		{"empty", ``, false},
		{"missing dsn", `{"channels":["x"]}`, false},
		{"missing channels", `{"dsn":"postgres://localhost/x"}`, false},
		{"empty channel", `{"dsn":"postgres://localhost/x","channels":[""]}`, false},
		{"ok", `{"dsn":"postgres://localhost/x","channels":["events"]}`, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := New().Validate(json.RawMessage(tc.body))
			if (err == nil) != tc.ok {
				t.Fatalf("ok = %v, err = %v", err == nil, err)
			}
		})
	}
}

func TestQuoteIdent(t *testing.T) {
	cases := map[string]string{
		"events":          `"events"`,
		"with space":      `"with space"`,
		`evil"--drop`:     `"evil""--drop"`,
		"":                `""`,
	}
	for in, want := range cases {
		got := pgxQuoteIdent(in)
		if got != want {
			t.Fatalf("pgxQuoteIdent(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestOverrideCredentials(t *testing.T) {
	rt := &fakeCtx{secrets: map[string]string{
		"postgres-username": "secretuser",
		"postgres-password": "secretpw",
	}}
	out, err := overrideCredentials("postgres://baduser:badpw@host:5432/db", rt)
	if err != nil {
		t.Fatal(err)
	}
	// secret should override both.
	if !contains(out, "secretuser") || !contains(out, "secretpw") {
		t.Fatalf("override failed: %s", out)
	}
	if contains(out, "baduser") {
		t.Fatalf("old user leaked: %s", out)
	}

	// Empty secrets → DSN unchanged.
	rt2 := &fakeCtx{secrets: map[string]string{}}
	dsn := "postgres://u:p@h/db"
	out2, err := overrideCredentials(dsn, rt2)
	if err != nil {
		t.Fatal(err)
	}
	if out2 != dsn {
		t.Fatalf("expected unchanged, got %s", out2)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
