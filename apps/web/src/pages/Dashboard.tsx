import { lazy, Suspense, useEffect, useMemo, useState } from 'react';
import {
  Activity,
  AlertTriangle,
  Clock,
  Database,
  Plug,
  Radio,
} from 'lucide-react';
import MetricCard from '../components/common/MetricCard';
import { useLiveStream } from '../hooks/useLiveStream';
import { getConnectors, getRecordCounts, getTimeseries } from '../api/sunny';
import type { ConnectorsResponse, TimeseriesBucket } from '../api/types';
import { formatNumber, formatTimeAgo } from '../utils/format';
import './Dashboard.css';

// Recharts is ~250 KB gzipped; lazy-load so the dashboard skeleton renders
// before the chart bundle arrives.
const ThroughputChart = lazy(() =>
  import('../components/charts/DashboardCharts').then((m) => ({ default: m.ThroughputChart })),
);
const SeverityBarChart = lazy(() =>
  import('../components/charts/DashboardCharts').then((m) => ({ default: m.SeverityBarChart })),
);

function ChartFallback() {
  return <div className="chart-skeleton">Loading chart…</div>;
}

// Format server-bucketed timeseries for the chart.
function formatBuckets(buckets: TimeseriesBucket[]): { time: string; count: number }[] {
  return buckets.map((b) => ({
    time: new Date(b.bucket).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
    count: b.count,
  }));
}

const SEVERITY_FILL: globalThis.Record<string, string> = {
  emergency: 'var(--severity-emergency)',
  critical: 'var(--severity-critical)',
  warning: 'var(--severity-warning)',
  info: 'var(--severity-info)',
};

export default function Dashboard() {
  const [conns, setConns] = useState<ConnectorsResponse | null>(null);
  const [series, setSeries] = useState<TimeseriesBucket[]>([]);
  const [counts, setCounts] = useState<globalThis.Record<string, number>>({});
  const stream = useLiveStream({ bufferSize: 500, replay: true });

  useEffect(() => {
    let cancelled = false;
    const tick = async () => {
      try {
        const [c, ts, ct] = await Promise.all([
          getConnectors(),
          getTimeseries({ bucketSeconds: 60 }),
          getRecordCounts(),
        ]);
        if (!cancelled) {
          setConns(c);
          setSeries(ts);
          setCounts(ct);
        }
      } catch {
        // ignore — sidebar shows the unhealthy state
      }
    };
    tick();
    const id = window.setInterval(tick, 5000);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, []);

  const running = useMemo(
    () => conns?.instances.filter((i) => i.state === 'running').length ?? 0,
    [conns],
  );
  const stopped = useMemo(
    () => (conns?.instances.length ?? 0) - running,
    [conns, running],
  );
  const buckets = useMemo(() => formatBuckets(series), [series]);
  const totalIngested = useMemo(
    () => Object.values(counts).reduce((s, n) => s + n, 0),
    [counts],
  );
  const severityCounts = useMemo(() => {
    const counts = { emergency: 0, critical: 0, warning: 0, info: 0 };
    for (const r of stream.records) {
      const s = (r.tags?.severity ?? '').toLowerCase();
      if (s in counts) counts[s as keyof typeof counts]++;
    }
    return counts;
  }, [stream.records]);

  const criticalRecent = useMemo(() => {
    return stream.records
      .filter((r) => {
        const s = (r.tags?.severity ?? '').toLowerCase();
        return s === 'emergency' || s === 'critical';
      })
      .slice(0, 8);
  }, [stream.records]);

  // First-boot empty state: no records ingested yet AND page has been
  // mounted less than 30 seconds (long enough that connectors have polled).
  const [pageMountedAt] = useState(() => Date.now());
  const [isOlderThan30s, setIsOlderThan30s] = useState(false);
  useEffect(() => {
    const id = window.setTimeout(
      () => setIsOlderThan30s(true),
      Math.max(0, 30_000 - (Date.now() - pageMountedAt)),
    );
    return () => window.clearTimeout(id);
  }, [pageMountedAt]);
  const showEmptyBanner =
    totalIngested === 0 && stream.total === 0 && !isOlderThan30s;

  return (
    <div className="dashboard">
      <div className="page-header">
        <div>
          <h1>Physical Observability</h1>
          <p className="page-subtitle">
            Real-time monitoring of physical infrastructure across the United States
          </p>
        </div>
        <div className="live-indicator">
          <span className={`status-dot ${stream.connected ? 'online pulse-dot' : 'offline'}`} />
          <span>{stream.connected ? 'LIVE' : 'OFFLINE'}</span>
          <Clock size={14} />
          <span>{new Date().toLocaleTimeString()}</span>
        </div>
      </div>

      {showEmptyBanner && (
        <div className="dashboard-empty-banner card">
          <Activity size={16} />
          <div>
            <strong>Connectors are starting up.</strong>
            <p>
              First poll usually completes in 5–60 s. Records will appear
              here as soon as they arrive. If nothing shows after a minute,
              check <code>sunny-cli connectors</code> for state.
            </p>
          </div>
        </div>
      )}

      <div className="metrics-grid">
        <MetricCard
          label="Running connectors"
          value={running}
          subtitle={`${stopped} stopped, ${conns?.types.length ?? 0} types available`}
          icon={<Plug size={16} />}
          accentColor="var(--status-good)"
        />
        <MetricCard
          label="Records / sec"
          value={stream.recordsPerSec.toFixed(1)}
          subtitle="5-second rolling window"
          icon={<Radio size={16} />}
          accentColor="var(--kafka-accent)"
        />
        <MetricCard
          label="Records ingested (total)"
          value={formatNumber(totalIngested)}
          subtitle={`+${formatNumber(stream.total)} this session`}
          icon={<Database size={16} />}
          accentColor="var(--accent)"
        />
        <MetricCard
          label="Critical alerts"
          value={severityCounts.emergency + severityCounts.critical}
          subtitle={`${severityCounts.warning} warnings in buffer`}
          icon={<AlertTriangle size={16} />}
          accentColor="var(--severity-emergency)"
        />
      </div>

      <div className="dashboard-grid">
        <div className="card dashboard-chart">
          <div className="card-header">
            <h3>
              <Radio size={16} />
              Ingest throughput, last hour (per-minute)
            </h3>
            <span className={`badge ${stream.connected ? 'badge-success' : 'badge-danger'}`}>
              <span className={`status-dot ${stream.connected ? 'online' : 'offline'}`} />
              {stream.connected ? 'streaming' : 'reconnecting…'}
            </span>
          </div>
          <Suspense fallback={<ChartFallback />}>
            <ThroughputChart data={buckets} />
          </Suspense>
        </div>

        <div className="card dashboard-alert-dist">
          <div className="card-header">
            <h3>
              <Activity size={16} />
              Severity distribution
            </h3>
          </div>
          <Suspense fallback={<ChartFallback />}>
            <SeverityBarChart
              data={[
                { name: 'Emergency', count: severityCounts.emergency, fill: SEVERITY_FILL.emergency },
                { name: 'Critical', count: severityCounts.critical, fill: SEVERITY_FILL.critical },
                { name: 'Warning', count: severityCounts.warning, fill: SEVERITY_FILL.warning },
                { name: 'Info', count: severityCounts.info, fill: SEVERITY_FILL.info },
              ]}
            />
          </Suspense>
        </div>

        <div className="card dashboard-alerts">
          <div className="card-header">
            <h3>
              <AlertTriangle size={16} />
              Recent critical / emergency records
            </h3>
          </div>
          <div className="alert-feed">
            {criticalRecent.length === 0 && (
              <p className="empty-state">No critical records in the live buffer.</p>
            )}
            {criticalRecent.map((r, i) => {
              const sev = (r.tags?.severity ?? 'info').toLowerCase();
              const payload = r.payload as globalThis.Record<string, unknown>;
              const headline =
                (payload?.headline as string) ||
                (payload?.event as string) ||
                (payload?.place as string) ||
                r.connectorId;
              return (
                <div key={i} className={`alert-feed-item severity-${sev}`}>
                  <div className="alert-feed-indicator">
                    <span
                      className={`status-dot pulse-dot`}
                      style={{ background: SEVERITY_FILL[sev] ?? 'var(--text-tertiary)' }}
                    />
                  </div>
                  <div className="alert-feed-content">
                    <div className="alert-feed-title">
                      <span className={`badge badge-${sev}`}>{sev}</span>
                      <span className="alert-feed-source">{r.connectorId}</span>
                    </div>
                    <p className="alert-feed-text">{headline}</p>
                    <div className="alert-feed-meta">
                      <span>{formatTimeAgo(r.timestamp)}</span>
                      {Object.entries(r.tags ?? {}).slice(0, 3).map(([k, v]) => (
                        <code key={k}>{k}={v}</code>
                      ))}
                    </div>
                  </div>
                </div>
              );
            })}
          </div>
        </div>

        <div className="card dashboard-pipeline">
          <div className="card-header">
            <h3>
              <Database size={16} />
              Ingest pipeline
            </h3>
          </div>
          <div className="pipeline-flow">
            <div className="pipeline-stage">
              <div className="pipeline-stage-header">
                <Plug size={14} />
                <span>Connectors</span>
              </div>
              <div className="pipeline-stage-stats">
                <div className="pipeline-stat">
                  <span className="pipeline-stat-value">{running}</span>
                  <span className="pipeline-stat-label">running</span>
                </div>
                <div className="pipeline-stat">
                  <span className="pipeline-stat-value">{conns?.types.length ?? 0}</span>
                  <span className="pipeline-stat-label">types</span>
                </div>
              </div>
            </div>
            <div className="pipeline-arrow">→</div>
            <div className="pipeline-stage">
              <div className="pipeline-stage-header">
                <Radio size={14} />
                <span>Bus</span>
              </div>
              <div className="pipeline-stage-stats">
                <div className="pipeline-stat">
                  <span className="pipeline-stat-value">{stream.recordsPerSec.toFixed(1)}</span>
                  <span className="pipeline-stat-label">rec/s</span>
                </div>
                <div className="pipeline-stat">
                  <span className="pipeline-stat-value">{stream.connected ? 'live' : 'offline'}</span>
                  <span className="pipeline-stat-label">status</span>
                </div>
              </div>
            </div>
            <div className="pipeline-arrow">→</div>
            <div className="pipeline-stage">
              <div className="pipeline-stage-header">
                <Database size={14} />
                <span>DuckDB</span>
              </div>
              <div className="pipeline-stage-stats">
                <div className="pipeline-stat">
                  <span className="pipeline-stat-value">{formatNumber(totalIngested)}</span>
                  <span className="pipeline-stat-label">total</span>
                </div>
                <div className="pipeline-stat">
                  <span className="pipeline-stat-value">embedded</span>
                  <span className="pipeline-stat-label">storage</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
