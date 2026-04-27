package httpapi

import (
	"net/http"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/go-chi/chi/v5"

	"github.com/sunny/sunny/apps/server/internal/bus"
	"github.com/sunny/sunny/apps/server/internal/connectors"
	"github.com/sunny/sunny/apps/server/internal/registry"
	sdk "github.com/sunny/sunny/packages/sdk-go"
)

var registryLoad = registry.Load

// MaxStreamConnections caps concurrent /api/stream WebSocket sessions.
// Prevents a fork-bomb-style DoS where one client opens thousands of
// connections. Configurable via SUNNY_MAX_STREAM_CONNS at startup.
var MaxStreamConnections int32 = 200

// streamCount is the global live counter. Incremented on Accept,
// decremented on Close.
var streamCount atomic.Int32

// connectorAPI groups handlers that need the runtime + bus.
type connectorAPI struct {
	runtime *connectors.Runtime
	bus     *bus.Bus
}

// listAll returns every registered connector type plus running instances.
// The UI uses this to populate /connectors.
func (a *connectorAPI) listAll(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"types":     connectors.Registered(),
		"instances": a.runtime.Statuses(),
	})
}

// listInstances returns just the running-instance array — much smaller than
// listAll. The sidebar polls this every 5 s.
func (a *connectorAPI) listInstances(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, a.runtime.Statuses())
}

func (a *connectorAPI) instance(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	st, ok := a.runtime.Status(id)
	if !ok {
		http.Error(w, "instance not found", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, st)
}


// stream is a WebSocket endpoint that tails the bus.
//
// Query params:
//
//	?connector=<id>   filter to one connector instance
//	?replay=1         send the ring buffer before live records
//
// The handler holds the connection open and writes every matching record
// as a JSON message. Slow clients drop records (the bus tracks drops).
func (a *connectorAPI) stream(w http.ResponseWriter, r *http.Request) {
	// Enforce the concurrent-connection cap before upgrading.
	if streamCount.Add(1) > MaxStreamConnections {
		streamCount.Add(-1)
		w.Header().Set("Retry-After", "5")
		http.Error(w, "stream connection cap reached; retry shortly", http.StatusServiceUnavailable)
		return
	}
	defer streamCount.Add(-1)

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// Permissive in dev; phase 6 hardens this when auth lands.
		InsecureSkipVerify: true,
	})
	if err != nil {
		return
	}
	defer conn.Close(websocket.StatusInternalError, "stream closed")

	wantConnector := r.URL.Query().Get("connector")
	replay := r.URL.Query().Get("replay") == "1"

	var filter func(sdk.Record) bool
	if wantConnector != "" {
		filter = func(rec sdk.Record) bool { return rec.ConnectorID == wantConnector }
	}

	sub := a.bus.Subscribe(filter, replay)
	defer sub.Close()

	ctx := r.Context()
	pingTicker := time.NewTicker(20 * time.Second)
	defer pingTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-pingTicker.C:
			if err := conn.Ping(ctx); err != nil {
				return
			}
		case rec, ok := <-sub.C():
			if !ok {
				return
			}
			if err := wsjson.Write(ctx, conn, rec); err != nil {
				return
			}
		}
	}
}

// mountConnectorRoutes wires the connector API onto the /api router.
func mountConnectorRoutes(r chi.Router, runtime *connectors.Runtime, b *bus.Bus) {
	api := &connectorAPI{runtime: runtime, bus: b}
	r.Get("/connectors", api.listAll)
	r.Get("/connectors/registry", api.registry)
	r.Get("/connectors/instances", api.listInstances)
	r.Get("/connectors/{id}", api.instance)
	r.Get("/stream", api.stream)
}

// registry returns the bundled connector registry document. The marketplace
// UI fetches this to show connectors that ARE NOT yet running but COULD be.
func (a *connectorAPI) registry(w http.ResponseWriter, _ *http.Request) {
	doc, err := registryLoad()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, doc)
}

// pushHandler dispatches /api/ingest/<id>/<rest> to the connector's push handler.
func pushHandler(rt *connectors.Runtime) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		h := rt.PushHandler(id)
		if h == nil {
			http.Error(w, "no push connector with id "+id, http.StatusNotFound)
			return
		}
		// Strip /api/ingest/<id> so the connector sees its mount-relative path.
		r2 := r.Clone(r.Context())
		prefix := "/api/ingest/" + id
		r2.URL.Path = "/" + chi.URLParam(r, "*")
		if r2.URL.Path == "/" && len(r.URL.Path) > len(prefix) {
			r2.URL.Path = r.URL.Path[len(prefix):]
		}
		h.ServeHTTP(w, r2)
	}
}
