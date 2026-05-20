// prometheus.go writes a Prometheus exposition-format endpoint at /metrics.
//
// We deliberately don't pull in github.com/prometheus/client_golang — the
// exposition format is two lines per metric and adding a 30-MB transitive
// dep for that is silly. If we ever need histograms with quantile
// estimation or other heavy-machinery we'll reconsider.
//
// Metrics exposed (initial set; Phase 1+ will grow this):
//
//   sunny_build_info{version,api_version}
//   sunny_records_total{connector="..."}
//   sunny_connector_state{instance="...",state="running|backoff|failed|stopped"}
//   sunny_connector_restarts_total{instance="..."}
//   sunny_alerts_total
//   sunny_alerts_pending  (un-acked)
//   sunny_dead_letters_total
//   process metrics: sunny_uptime_seconds, go_goroutines, go_memstats_*
//
// The endpoint is unauthenticated by default because that's what Prometheus
// scrapers expect; gate it via the firewall or set SUNNY_METRICS_REQUIRE_AUTH=1
// to bring it behind the standard auth middleware.
package httpapi

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/sunny/sunny/apps/server/internal/alerts"
	"github.com/sunny/sunny/apps/server/internal/connectors"
	"github.com/sunny/sunny/apps/server/internal/storage"
)

// startTime is captured at process start to power sunny_uptime_seconds.
var startTime = time.Now()

type prometheusAPI struct {
	runtime *connectors.Runtime
	store   storage.Storage
	dlq     alerts.DeadLetterStore
}

func (p *prometheusAPI) handle(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	p.write(r.Context(), w)
}

func (p *prometheusAPI) write(ctx context.Context, w io.Writer) {
	mw := &metricWriter{w: w}

	// Build info — single metric, labels carry the version.
	mw.help("sunny_build_info", "Sunny build metadata. Always 1.")
	mw.typ("sunny_build_info", "gauge")
	mw.line("sunny_build_info", map[string]string{
		"version":     Version,
		"api_version": fmt.Sprintf("%d", APIVersion),
	}, 1)

	// Uptime.
	mw.help("sunny_uptime_seconds", "Seconds since process start.")
	mw.typ("sunny_uptime_seconds", "counter")
	mw.line("sunny_uptime_seconds", nil, time.Since(startTime).Seconds())

	// Go runtime.
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	mw.help("go_goroutines", "Number of goroutines currently running.")
	mw.typ("go_goroutines", "gauge")
	mw.line("go_goroutines", nil, float64(runtime.NumGoroutine()))
	mw.help("go_memstats_alloc_bytes", "Bytes of heap objects allocated.")
	mw.typ("go_memstats_alloc_bytes", "gauge")
	mw.line("go_memstats_alloc_bytes", nil, float64(ms.Alloc))
	mw.help("go_memstats_heap_inuse_bytes", "Bytes in in-use heap spans.")
	mw.typ("go_memstats_heap_inuse_bytes", "gauge")
	mw.line("go_memstats_heap_inuse_bytes", nil, float64(ms.HeapInuse))

	// Connector instance states + restarts.
	if p.runtime != nil {
		states := p.runtime.Statuses()
		// Sort for deterministic output (helps tests + diff-friendly scrapes).
		sort.Slice(states, func(i, j int) bool { return states[i].InstanceID < states[j].InstanceID })
		mw.help("sunny_connector_state", "Current state per connector instance, encoded one-hot.")
		mw.typ("sunny_connector_state", "gauge")
		for _, st := range states {
			for _, s := range []string{"running", "backoff", "failed", "stopped", "starting"} {
				v := 0.0
				if string(st.State) == s {
					v = 1
				}
				mw.line("sunny_connector_state", map[string]string{
					"instance": st.InstanceID, "state": s,
				}, v)
			}
		}
		mw.help("sunny_connector_restarts_total", "Restart count per connector instance.")
		mw.typ("sunny_connector_restarts_total", "counter")
		for _, st := range states {
			mw.line("sunny_connector_restarts_total", map[string]string{"instance": st.InstanceID}, float64(st.Restarts))
		}
	}

	// Per-connector record totals.
	if p.store != nil {
		if counts, err := p.store.CountByConnector(ctx); err == nil {
			mw.help("sunny_records_total", "Total ingested records per connector.")
			mw.typ("sunny_records_total", "counter")
			keys := make([]string, 0, len(counts))
			for k := range counts {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				mw.line("sunny_records_total", map[string]string{"connector": k}, float64(counts[k]))
			}
		}
		if list, err := p.store.ListAlerts(ctx, 1000); err == nil {
			mw.help("sunny_alerts_total", "Alerts visible to the API.")
			mw.typ("sunny_alerts_total", "gauge")
			mw.line("sunny_alerts_total", nil, float64(len(list)))
			pending := 0
			for _, a := range list {
				if a.Acked == nil {
					pending++
				}
			}
			mw.help("sunny_alerts_pending", "Alerts visible to the API that have not been acknowledged.")
			mw.typ("sunny_alerts_pending", "gauge")
			mw.line("sunny_alerts_pending", nil, float64(pending))
		}
	}

	// Dead letters.
	if p.dlq != nil {
		if dl, err := p.dlq.ListDeadLetters(ctx, 1000); err == nil {
			mw.help("sunny_dead_letters_total", "Alerts that exhausted notifier retries.")
			mw.typ("sunny_dead_letters_total", "gauge")
			mw.line("sunny_dead_letters_total", nil, float64(len(dl)))
		}
	}
}

// metricWriter writes one Prometheus exposition-format metric line at a
// time. It tracks emitted # HELP/# TYPE lines so each metric name only
// gets them once.
type metricWriter struct {
	w        io.Writer
	helped   map[string]bool
	typed    map[string]bool
}

func (m *metricWriter) help(name, desc string) {
	if m.helped == nil {
		m.helped = map[string]bool{}
	}
	if m.helped[name] {
		return
	}
	m.helped[name] = true
	fmt.Fprintf(m.w, "# HELP %s %s\n", name, desc)
}

func (m *metricWriter) typ(name, t string) {
	if m.typed == nil {
		m.typed = map[string]bool{}
	}
	if m.typed[name] {
		return
	}
	m.typed[name] = true
	fmt.Fprintf(m.w, "# TYPE %s %s\n", name, t)
}

func (m *metricWriter) line(name string, labels map[string]string, v float64) {
	if len(labels) == 0 {
		fmt.Fprintf(m.w, "%s %s\n", name, formatFloat(v))
		return
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	sb.WriteString(name)
	sb.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(k)
		sb.WriteByte('=')
		sb.WriteByte('"')
		sb.WriteString(escapeLabel(labels[k]))
		sb.WriteByte('"')
	}
	sb.WriteByte('}')
	fmt.Fprintf(m.w, "%s %s\n", sb.String(), formatFloat(v))
}

func escapeLabel(s string) string {
	// Per https://prometheus.io/docs/instrumenting/exposition_formats/ —
	// label values escape backslash, double quote, newline.
	if !strings.ContainsAny(s, `\"`+"\n") {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func formatFloat(v float64) string {
	// Prometheus accepts standard %g — integers come out clean ("1"
	// instead of "1.000000"), floats keep precision.
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.6f", v), "0"), ".")
}
