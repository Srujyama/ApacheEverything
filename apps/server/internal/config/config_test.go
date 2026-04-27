package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMissingFileFallsBackToDefault(t *testing.T) {
	c, err := Load(filepath.Join(t.TempDir(), "nope.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Addr != ":3000" || c.DataDir != "./data" {
		t.Fatalf("default config addr/dataDir wrong: %+v", c)
	}
	// Default boots with hello + the no-auth real connectors.
	wantTypes := map[string]bool{
		"hello":               true,
		"usgs-earthquakes":    true,
		"noaa-weather-alerts": true,
		"usgs-water":          true,
	}
	if len(c.Connectors) != len(wantTypes) {
		t.Fatalf("default config has %d connectors, want %d", len(c.Connectors), len(wantTypes))
	}
	for _, cc := range c.Connectors {
		if !wantTypes[cc.Type] {
			t.Fatalf("unexpected default connector type %q", cc.Type)
		}
	}
}

func TestLoadParsesConnectors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sunny.config.yaml")
	body := `addr: ":3030"
connectors:
  - id: usgs
    type: usgs-earthquakes
    config:
      pollEvery: 30s
      minMagnitude: 2.5
  - id: hello-1
    type: hello
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.Addr != ":3030" {
		t.Fatalf("addr = %q", c.Addr)
	}
	specs, err := c.ToInstanceSpecs()
	if err != nil {
		t.Fatalf("ToInstanceSpecs: %v", err)
	}
	if len(specs) != 2 {
		t.Fatalf("want 2 specs, got %d", len(specs))
	}
	if !strings.Contains(string(specs[0].Config), `"minMagnitude":2.5`) {
		t.Fatalf("usgs config missing minMagnitude: %s", specs[0].Config)
	}
}

func TestDuplicateIDRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.yaml")
	body := `connectors:
  - id: dup
    type: hello
  - id: dup
    type: hello
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.ToInstanceSpecs(); err == nil {
		t.Fatal("expected duplicate-id error")
	}
}
