// Package connectors is the runtime side of the connector SDK.
//
// At program init time, every linked connector calls Register(). The runtime
// reads the config (sunny.config.yaml) and starts an instance for each entry,
// each backed by one of the registered connector types.
package connectors

import (
	"fmt"
	"sort"
	"sync"

	sdk "github.com/sunny/sunny/packages/sdk-go"
)

var (
	regMu sync.RWMutex
	reg   = map[string]sdk.Connector{}
)

// Register registers a connector type by its manifest ID. Intended to be
// called from package init() of connector packages — see example_hello for
// the pattern. Panics on duplicate registration so collisions are caught at
// startup, not in production.
func Register(c sdk.Connector) {
	m := c.Manifest()
	if m.ID == "" {
		panic("connectors.Register: manifest has empty ID")
	}
	regMu.Lock()
	defer regMu.Unlock()
	if _, exists := reg[m.ID]; exists {
		panic(fmt.Sprintf("connectors.Register: duplicate ID %q", m.ID))
	}
	reg[m.ID] = c
}

// Lookup returns the registered connector for id, or false if none.
func Lookup(id string) (sdk.Connector, bool) {
	regMu.RLock()
	defer regMu.RUnlock()
	c, ok := reg[id]
	return c, ok
}

// Registered returns all registered manifests, sorted by ID.
func Registered() []sdk.Manifest {
	regMu.RLock()
	defer regMu.RUnlock()
	out := make([]sdk.Manifest, 0, len(reg))
	for _, c := range reg {
		out = append(out, c.Manifest())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
