package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"golang.org/x/crypto/bcrypt"

	"github.com/sunny/sunny/apps/server/internal/auth"
	"github.com/sunny/sunny/apps/server/internal/bus"
	"github.com/sunny/sunny/apps/server/internal/connectors"
	"github.com/sunny/sunny/apps/server/internal/storage"
	sdk "github.com/sunny/sunny/packages/sdk-go"
)

func quietLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func setup(t *testing.T, authMgr *auth.Manager) (*httptest.Server, storage.Storage, *bus.Bus) {
	t.Helper()
	store, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })

	b := bus.New(64, 64)
	rt := connectors.NewRuntime(b, quietLogger(), connectors.EnvSecrets{}, store)

	r := NewRouter(Deps{Logger: quietLogger(), Runtime: rt, Bus: b, Storage: store, Auth: authMgr})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, store, b
}

func mustGet(t *testing.T, url string) *http.Response {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

func mustPost(t *testing.T, url, body string) *http.Response {
	t.Helper()
	res, err := http.Post(url, "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

func mustDo(t *testing.T, req *http.Request) *http.Response {
	t.Helper()
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", req.Method, req.URL, err)
	}
	t.Cleanup(func() { _ = res.Body.Close() })
	return res
}

func TestHealthPublic(t *testing.T) {
	srv, _, _ := setup(t, nil)
	res := mustGet(t, srv.URL+"/api/health")
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestVersionShape(t *testing.T) {
	srv, _, _ := setup(t, nil)
	res := mustGet(t, srv.URL+"/api/version")
	var body map[string]string
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body["version"] != Version {
		t.Fatalf("version = %q, want %q", body["version"], Version)
	}
}

func TestRegistryEndpoint(t *testing.T) {
	srv, _, _ := setup(t, nil)
	res := mustGet(t, srv.URL+"/api/connectors/registry")
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"id":"webhook"`) {
		t.Fatalf("registry missing webhook entry: %s", body)
	}
}

func TestRecordsEndpointReadsStorage(t *testing.T) {
	srv, store, _ := setup(t, nil)
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := store.Write(context.Background(), []sdk.Record{
		{Timestamp: now, ConnectorID: "test-c", Payload: json.RawMessage(`{"v":1}`)},
		{Timestamp: now.Add(time.Second), ConnectorID: "test-c", Payload: json.RawMessage(`{"v":2}`)},
	}); err != nil {
		t.Fatal(err)
	}

	res := mustGet(t, srv.URL+"/api/records?connector=test-c&limit=10")
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	var got []sdk.Record
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d records, want 2", len(got))
	}
}

func TestRecordsCounts(t *testing.T) {
	srv, store, _ := setup(t, nil)
	ctx := context.Background()
	if err := store.Write(ctx, []sdk.Record{
		{Timestamp: time.Now(), ConnectorID: "a", Payload: json.RawMessage(`{}`)},
		{Timestamp: time.Now(), ConnectorID: "a", Payload: json.RawMessage(`{}`)},
		{Timestamp: time.Now(), ConnectorID: "b", Payload: json.RawMessage(`{}`)},
	}); err != nil {
		t.Fatal(err)
	}
	res := mustGet(t, srv.URL+"/api/records/counts")
	var got map[string]int64
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got["a"] != 2 || got["b"] != 1 {
		t.Fatalf("counts = %+v", got)
	}
}

func TestTimeseriesEndpoint(t *testing.T) {
	srv, store, _ := setup(t, nil)
	base := time.Now().UTC().Truncate(time.Minute)
	for i := 0; i < 5; i++ {
		if err := store.Write(context.Background(), []sdk.Record{
			{Timestamp: base.Add(time.Duration(i) * 10 * time.Second), ConnectorID: "c", Payload: json.RawMessage(`{}`)},
		}); err != nil {
			t.Fatal(err)
		}
	}
	url := srv.URL + "/api/timeseries?bucket=60&from=" + base.Format(time.RFC3339) +
		"&to=" + base.Add(time.Minute).Format(time.RFC3339)
	res := mustGet(t, url)
	var got []storage.TimeseriesBucket
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Count != 5 {
		t.Fatalf("got %+v, want 1 bucket of 5", got)
	}
}

func TestAlertsRulesCRUD(t *testing.T) {
	srv, _, _ := setup(t, nil)

	res := mustPost(t, srv.URL+"/api/alerts/rules",
		`{"name":"test","enabled":true,"connectorId":"c1"}`)
	if res.StatusCode != 200 {
		t.Fatalf("post status %d", res.StatusCode)
	}
	var rule map[string]any
	if err := json.NewDecoder(res.Body).Decode(&rule); err != nil {
		t.Fatal(err)
	}
	id, _ := rule["id"].(string)
	if id == "" {
		t.Fatal("rule id empty")
	}

	res2 := mustGet(t, srv.URL+"/api/alerts/rules")
	got, err := io.ReadAll(res2.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), id) {
		t.Fatalf("list missing rule id %q", id)
	}

	req, err := http.NewRequest("DELETE", srv.URL+"/api/alerts/rules/"+id, nil)
	if err != nil {
		t.Fatal(err)
	}
	res3 := mustDo(t, req)
	if res3.StatusCode != 204 {
		t.Fatalf("delete status %d", res3.StatusCode)
	}
}

func TestAuthDisabledByDefault(t *testing.T) {
	srv, _, _ := setup(t, nil)
	res := mustGet(t, srv.URL+"/api/connectors")
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 with no auth, got %d", res.StatusCode)
	}
}

func TestAuthEnabledRequiresCookie(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("p"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	mgr, err := auth.NewManager(string(hash), "abcdefghijklmnopqrstuvwxyz123456", "")
	if err != nil {
		t.Fatal(err)
	}
	srv, _, _ := setup(t, mgr)

	res := mustGet(t, srv.URL+"/api/health")
	if res.StatusCode != 200 {
		t.Fatalf("health blocked: %d", res.StatusCode)
	}

	res2 := mustGet(t, srv.URL+"/api/connectors")
	if res2.StatusCode != 401 {
		t.Fatalf("expected 401 without cookie, got %d", res2.StatusCode)
	}

	loginRes := mustPost(t, srv.URL+"/api/auth/login", `{"password":"p"}`)
	if loginRes.StatusCode != 200 {
		t.Fatalf("login status %d", loginRes.StatusCode)
	}
	cookies := loginRes.Cookies()
	if len(cookies) == 0 {
		t.Fatal("no session cookie")
	}

	req, err := http.NewRequest("GET", srv.URL+"/api/connectors", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(cookies[0])
	res3 := mustDo(t, req)
	if res3.StatusCode != 200 {
		t.Fatalf("connectors blocked even with cookie: %d", res3.StatusCode)
	}
}

func TestPushIngestBypassesAuth(t *testing.T) {
	// Even with auth on, push ingest must work without a session cookie.
	hash, err := bcrypt.GenerateFromPassword([]byte("p"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	mgr, _ := auth.NewManager(string(hash), "k123456789012345678901234567890123", "")
	srv, _, _ := setup(t, mgr)

	// /api/connectors is gated.
	res := mustGet(t, srv.URL+"/api/connectors")
	if res.StatusCode != 401 {
		t.Fatalf("expected /connectors to require auth: %d", res.StatusCode)
	}
	// /api/ingest/<missing> returns 404 (not 401) — proves the middleware
	// didn't intercept.
	res2 := mustPost(t, srv.URL+"/api/ingest/nonexistent/", "{}")
	if res2.StatusCode != 404 {
		t.Fatalf("expected /ingest/nonexistent → 404 (not auth-blocked), got %d", res2.StatusCode)
	}
}

func TestSPAFallback(t *testing.T) {
	srv, _, _ := setup(t, nil)
	res := mustGet(t, srv.URL+"/random/spa/route")
	if res.StatusCode != 200 {
		t.Fatalf("SPA fallback status %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("expected html, got %q", ct)
	}
}

func TestExportCSVEndpoint(t *testing.T) {
	srv, store, _ := setup(t, nil)
	if err := store.Write(context.Background(), []sdk.Record{
		{Timestamp: time.Now(), ConnectorID: "exp-c", Payload: json.RawMessage(`{"v":1}`)},
		{Timestamp: time.Now(), ConnectorID: "exp-c", Payload: json.RawMessage(`{"v":2}`)},
	}); err != nil {
		t.Fatal(err)
	}
	res := mustGet(t, srv.URL+"/api/export?format=csv&connector=exp-c")
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Fatalf("content-type %q", ct)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "exp-c") {
		t.Fatalf("CSV missing data: %s", body)
	}
	// Header row + ≥ 2 data rows.
	if strings.Count(string(body), "\n") < 3 {
		t.Fatalf("too few rows: %s", body)
	}
}

func TestExportRejectsInvalidFormat(t *testing.T) {
	srv, _, _ := setup(t, nil)
	res := mustGet(t, srv.URL+"/api/export?format=xml")
	if res.StatusCode != 400 {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestQueryEndpointSelectOK(t *testing.T) {
	srv, store, _ := setup(t, nil)
	if err := store.Write(context.Background(), []sdk.Record{
		{Timestamp: time.Now(), ConnectorID: "qry-c", Payload: json.RawMessage(`{}`)},
		{Timestamp: time.Now(), ConnectorID: "qry-c", Payload: json.RawMessage(`{}`)},
	}); err != nil {
		t.Fatal(err)
	}
	res := mustPost(t, srv.URL+"/api/query",
		`{"sql":"SELECT connector_id, COUNT(*) AS n FROM events GROUP BY connector_id"}`)
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	var got map[string]any
	if err := json.NewDecoder(res.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if cols, _ := got["columns"].([]any); len(cols) != 2 {
		t.Fatalf("columns = %v", got["columns"])
	}
	if rc, _ := got["rowCount"].(float64); rc < 1 {
		t.Fatalf("rowCount = %v", got["rowCount"])
	}
}

func TestQueryEndpointRejectsMultistatement(t *testing.T) {
	srv, _, _ := setup(t, nil)
	res := mustPost(t, srv.URL+"/api/query", `{"sql":"SELECT 1; SELECT 2"}`)
	if res.StatusCode != 400 {
		t.Fatalf("status %d, want 400", res.StatusCode)
	}
}

func TestQueryEndpointRejectsDDL(t *testing.T) {
	srv, _, _ := setup(t, nil)
	res := mustPost(t, srv.URL+"/api/query", `{"sql":"DROP TABLE events"}`)
	if res.StatusCode != 400 {
		t.Fatalf("status %d, want 400", res.StatusCode)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	srv, store, _ := setup(t, nil)
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		_ = store.Write(context.Background(), []sdk.Record{
			{Timestamp: now.Add(-time.Duration(i) * time.Minute), ConnectorID: "metric-c", Payload: json.RawMessage(`{}`)},
		})
	}
	res := mustGet(t, srv.URL+"/api/connectors/metric-c/metrics")
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	var m map[string]any
	if err := json.NewDecoder(res.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	if m["totalRecords"].(float64) != 5 {
		t.Fatalf("totalRecords = %v, want 5", m["totalRecords"])
	}
	if m["lastRecordAt"] == nil {
		t.Fatal("lastRecordAt not populated")
	}
	if rate, _ := m["ratePerMinLastHour"].(float64); rate <= 0 {
		t.Fatalf("ratePerMinLastHour = %v, want > 0", rate)
	}
}

func TestMetricsEndpointMissingInstance(t *testing.T) {
	srv, _, _ := setup(t, nil)
	res := mustGet(t, srv.URL+"/api/connectors/no-such-instance/metrics")
	if res.StatusCode != 200 {
		t.Fatalf("expected 200 even for missing instance, got %d", res.StatusCode)
	}
	var m map[string]any
	_ = json.NewDecoder(res.Body).Decode(&m)
	if m["state"] != "unknown" {
		t.Fatalf("state = %v, want unknown", m["state"])
	}
	if m["totalRecords"].(float64) != 0 {
		t.Fatalf("totalRecords = %v, want 0", m["totalRecords"])
	}
}

func TestBackupEndpoint(t *testing.T) {
	// Need a real data dir for this test, not the in-memory store path.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignored.duckdb.wal"), []byte("wal"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := storage.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	b := bus.New(8, 8)
	rt := connectors.NewRuntime(b, quietLogger(), connectors.EnvSecrets{}, store)
	r := NewRouter(Deps{
		Logger: quietLogger(), Runtime: rt, Bus: b, Storage: store,
		DataDir: dir,
	})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	res := mustGet(t, srv.URL+"/api/backup")
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); ct != "application/gzip" {
		t.Fatalf("content-type = %q", ct)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) < 16 {
		t.Fatalf("backup too small: %d bytes", len(body))
	}
	// Gzip magic bytes.
	if body[0] != 0x1f || body[1] != 0x8b {
		t.Fatalf("not gzip: %x %x", body[0], body[1])
	}
}

func TestKubernetesProbes(t *testing.T) {
	srv, _, _ := setup(t, nil)

	res := mustGet(t, srv.URL+"/healthz")
	if res.StatusCode != 200 {
		t.Fatalf("/healthz status %d", res.StatusCode)
	}

	res2 := mustGet(t, srv.URL+"/readyz")
	if res2.StatusCode != 200 {
		t.Fatalf("/readyz status %d", res2.StatusCode)
	}
}

func TestProbeBypassesAuth(t *testing.T) {
	hash, _ := bcrypt.GenerateFromPassword([]byte("p"), bcrypt.MinCost)
	mgr, _ := auth.NewManager(string(hash), "k123456789012345678901234567890123", "")
	srv, _, _ := setup(t, mgr)

	res := mustGet(t, srv.URL+"/healthz")
	if res.StatusCode != 200 {
		t.Fatalf("healthz blocked by auth: %d", res.StatusCode)
	}
}

func TestOpenAPISpec(t *testing.T) {
	srv, _, _ := setup(t, nil)
	res := mustGet(t, srv.URL+"/api/openapi.yaml")
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.HasPrefix(ct, "application/yaml") {
		t.Fatalf("content-type %q", ct)
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"openapi: 3.1.0", "/api/connectors", "/api/query", "/api/export"} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("spec missing %q", want)
		}
	}
}

func TestAPIDocs(t *testing.T) {
	srv, _, _ := setup(t, nil)
	res := mustGet(t, srv.URL+"/api/docs")
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	if !strings.Contains(string(body), "rapi-doc") {
		t.Fatalf("docs page missing rapidoc embed: %s", body)
	}
}

func TestStreamConnectionCap(t *testing.T) {
	srv, _, _ := setup(t, nil)

	old := MaxStreamConnections
	MaxStreamConnections = 1
	t.Cleanup(func() { MaxStreamConnections = old; streamCount.Store(0) })

	// Open 1 stream connection (the cap).
	wsURL := strings.Replace(srv.URL, "http://", "ws://", 1) + "/api/stream"
	c1, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err != nil {
		t.Fatalf("first dial: %v", err)
	}
	t.Cleanup(func() { c1.Close(websocket.StatusNormalClosure, "") })

	// Second should fail with 503.
	res, err := http.Get(srv.URL + "/api/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("over-cap stream returned %d, want 503", res.StatusCode)
	}
	if res.Header.Get("Retry-After") == "" {
		t.Fatal("missing Retry-After")
	}
}
