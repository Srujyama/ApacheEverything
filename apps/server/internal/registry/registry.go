// Package registry serves the bundled connector-registry document.
//
// In v1 we ship a static, baked-in registry. In v1.x the server can also
// pull from a remote URL (e.g. https://registry.sunny.dev/registry.json)
// and merge the results. For now: just expose what's in registry.json.
package registry

import (
	_ "embed"
	"encoding/json"
	"errors"
)

//go:embed registry.json
var bundled []byte

// Document is the parsed shape of registry.json. We don't validate
// against the JSON Schema at runtime — the schema is for editors and CI.
type Document struct {
	Version    string  `json:"version"`
	Updated    string  `json:"updated,omitempty"`
	Connectors []Entry `json:"connectors"`
}

type Entry struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	Description  string          `json:"description,omitempty"`
	Category     string          `json:"category"`
	Mode         string          `json:"mode"`
	Version      string          `json:"version,omitempty"`
	Source       Source          `json:"source"`
	Secrets      []Secret        `json:"secrets,omitempty"`
	ConfigSchema json.RawMessage `json:"configSchema,omitempty"`
	Homepage     string          `json:"homepage,omitempty"`
	License      string          `json:"license,omitempty"`
	Maintainers  []string        `json:"maintainers,omitempty"`
	Verified     bool            `json:"verified,omitempty"`
}

type Source struct {
	Type     string `json:"type"`
	Module   string `json:"module,omitempty"`
	Endpoint string `json:"endpoint,omitempty"`
}

type Secret struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	EnvExample  string `json:"envExample,omitempty"`
	ObtainURL   string `json:"obtainUrl,omitempty"`
}

// Load returns the bundled registry document.
func Load() (Document, error) {
	if len(bundled) == 0 {
		return Document{}, errors.New("no bundled registry")
	}
	var d Document
	if err := json.Unmarshal(bundled, &d); err != nil {
		return Document{}, err
	}
	return d, nil
}
