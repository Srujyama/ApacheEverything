// Package config loads sunny.config.yaml.
//
// Example file:
//
//	addr: ":3000"
//	connectors:
//	  - id: hello-1
//	    type: hello
//	    config: {}
//
// If the file is absent, Load returns DefaultConfig() — which runs the hello
// connector on :3000. That makes a fresh `./bin/sunny` Just Work for demos.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/sunny/sunny/apps/server/internal/connectors"
)

// Config is the on-disk shape of sunny.config.yaml.
type Config struct {
	Addr       string            `yaml:"addr"`
	DataDir    string            `yaml:"dataDir"`
	Connectors []ConnectorConfig `yaml:"connectors"`
}

// ConnectorConfig describes one instance to start.
type ConnectorConfig struct {
	ID     string    `yaml:"id"`
	Type   string    `yaml:"type"`
	Config yaml.Node `yaml:"config"` // arbitrary; converted to JSON for the connector
}

// DefaultConfig returns the baked-in default used when no config file exists.
// It starts the no-auth real-data connectors so a fresh install shows real
// U.S. infrastructure data immediately, plus hello so the runtime is
// obviously alive even if all upstream APIs are down.
func DefaultConfig() Config {
	return Config{
		Addr:    ":3000",
		DataDir: "./data",
		Connectors: []ConnectorConfig{
			{ID: "hello-1", Type: "hello"},
			{ID: "earthquakes", Type: "usgs-earthquakes"},
			{ID: "weather-alerts", Type: "noaa-weather-alerts"},
			{ID: "river-gauges", Type: "usgs-water"},
		},
	}
}

// Load reads path, returning DefaultConfig() if it doesn't exist.
func Load(path string) (Config, error) {
	if path == "" {
		return DefaultConfig(), nil
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return DefaultConfig(), nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if c.Addr == "" {
		c.Addr = ":3000"
	}
	if c.DataDir == "" {
		c.DataDir = "./data"
	}
	return c, nil
}

// ToInstanceSpecs converts the parsed config into runtime instance specs.
// Each connector's config block is re-encoded as JSON so the connector can
// unmarshal it with its own struct types.
func (c Config) ToInstanceSpecs() ([]connectors.InstanceSpec, error) {
	specs := make([]connectors.InstanceSpec, 0, len(c.Connectors))
	seen := map[string]bool{}
	for i, cc := range c.Connectors {
		if cc.ID == "" {
			return nil, fmt.Errorf("connectors[%d]: id is required", i)
		}
		if cc.Type == "" {
			return nil, fmt.Errorf("connectors[%d] (%s): type is required", i, cc.ID)
		}
		if seen[cc.ID] {
			return nil, fmt.Errorf("connectors[%d]: duplicate id %q", i, cc.ID)
		}
		seen[cc.ID] = true

		raw := json.RawMessage(`{}`)
		if !cc.Config.IsZero() {
			b, err := yamlNodeToJSON(cc.Config)
			if err != nil {
				return nil, fmt.Errorf("connectors[%d] (%s): %w", i, cc.ID, err)
			}
			raw = b
		}
		specs = append(specs, connectors.InstanceSpec{
			InstanceID: cc.ID,
			Type:       cc.Type,
			Config:     raw,
		})
	}
	return specs, nil
}

// yamlNodeToJSON converts an arbitrary YAML node to JSON. We can't just
// json.Marshal the yaml.Node directly because yaml.v3 nodes carry tag/style
// metadata that doesn't round-trip — Decode+Marshal is the supported path.
func yamlNodeToJSON(n yaml.Node) (json.RawMessage, error) {
	var v any
	if err := n.Decode(&v); err != nil {
		return nil, err
	}
	v = normalizeKeys(v)
	return json.Marshal(v)
}

// normalizeKeys converts map[interface{}]interface{} (yaml.v3's default for
// some nested maps) into map[string]interface{} so json.Marshal accepts it.
func normalizeKeys(v any) any {
	switch t := v.(type) {
	case map[any]any:
		m := make(map[string]any, len(t))
		for k, vv := range t {
			m[fmt.Sprint(k)] = normalizeKeys(vv)
		}
		return m
	case map[string]any:
		for k, vv := range t {
			t[k] = normalizeKeys(vv)
		}
		return t
	case []any:
		for i, vv := range t {
			t[i] = normalizeKeys(vv)
		}
		return t
	default:
		return v
	}
}
