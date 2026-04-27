import { useEffect, useMemo, useState } from 'react';
import { Radio, Activity, Pause, Database, RefreshCcw } from 'lucide-react';
import { useLiveStream } from '../hooks/useLiveStream';
import { getConnectors, getRecordCounts } from '../api/sunny';
import type {
  ConnectorsResponse,
  InstanceStatus,
  Manifest,
  Record as SunnyRecord,
} from '../api/types';
import { formatNumber, formatTimeAgo } from '../utils/format';
import './DataStreams.css';

interface InstanceStats {
  instance: InstanceStatus;
  manifest?: Manifest;
  totalRecords: number;
  lastRecordTs?: string;
  recentRate: number; // records/sec from live buffer
}

export default function DataStreams() {
  const [conns, setConns] = useState<ConnectorsResponse | null>(null);
  const [counts, setCounts] = useState<Map<string, number>>(new Map());
  const [selected, setSelected] = useState<string | undefined>();
  const stream = useLiveStream({ bufferSize: 1000, replay: true, connector: selected });

  // Load connector list and per-instance record counts (server-aggregated).
  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      try {
        const [c, ct] = await Promise.all([getConnectors(), getRecordCounts()]);
        if (cancelled) return;
        setConns(c);
        const next = new Map<string, number>();
        for (const [k, v] of Object.entries(ct)) next.set(k, v);
        setCounts(next);
      } catch {
        // ignore
      }
    };
    load();
    const id = window.setInterval(load, 5000);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, []);

  const stats: InstanceStats[] = useMemo(() => {
    if (!conns) return [];
    return conns.instances.map((inst) => {
      const manifest = conns.types.find((t) => t.id === inst.type);
      const live = stream.records.filter((r) => r.connectorId === inst.instanceId);
      const lastRecordTs = live[0]?.timestamp;

      const now = Date.now();
      const lastMin = live.filter(
        (r) => now - new Date(r.timestamp).getTime() <= 60000,
      ).length;
      return {
        instance: inst,
        manifest,
        totalRecords: counts.get(inst.instanceId) ?? 0,
        lastRecordTs,
        recentRate: lastMin / 60,
      };
    });
  }, [conns, counts, stream.records]);

  const filtered = useMemo(() => {
    if (!selected) return stream.records;
    return stream.records.filter((r) => r.connectorId === selected);
  }, [selected, stream.records]);

  return (
    <div className="data-streams">
      <div className="page-header">
        <div>
          <h1>Data Streams</h1>
          <p className="page-subtitle">
            Live view of every connector instance and the records it's producing
          </p>
        </div>
        <div className="live-indicator">
          <span className={`status-dot ${stream.connected ? 'online pulse-dot' : 'offline'}`} />
          <span>{stream.connected ? 'streaming' : 'reconnecting'}</span>
        </div>
      </div>

      <div className="streams-grid">
        <div className="card streams-list">
          <div className="card-header">
            <h3>
              <Radio size={16} />
              Connector instances
            </h3>
            {selected && (
              <button
                className="text-link"
                onClick={() => setSelected(undefined)}
                title="Clear filter"
              >
                <RefreshCcw size={12} /> all
              </button>
            )}
          </div>
          <div className="streams-rows">
            {stats.length === 0 && (
              <p className="empty-state">No connector instances running yet.</p>
            )}
            {stats.map((s) => (
              <button
                key={s.instance.instanceId}
                className={`streams-row ${selected === s.instance.instanceId ? 'streams-row-active' : ''}`}
                onClick={() =>
                  setSelected(
                    selected === s.instance.instanceId ? undefined : s.instance.instanceId,
                  )
                }
              >
                <div className="streams-row-head">
                  <span className="streams-row-name">{s.instance.instanceId}</span>
                  <span className={`badge state-${s.instance.state}`}>
                    {s.instance.state}
                  </span>
                </div>
                <div className="streams-row-meta">
                  <code>{s.instance.type}</code>
                  {s.manifest && <code>{s.manifest.mode}</code>}
                  {s.instance.restarts > 0 && (
                    <span className="streams-row-warn">restarts: {s.instance.restarts}</span>
                  )}
                </div>
                <div className="streams-row-stats">
                  <span>
                    <Database size={11} /> {formatNumber(s.totalRecords)}
                  </span>
                  <span>
                    <Activity size={11} /> {s.recentRate.toFixed(2)}/s
                  </span>
                  <span>
                    <Pause size={11} />{' '}
                    {s.lastRecordTs ? formatTimeAgo(s.lastRecordTs) : '—'}
                  </span>
                </div>
                {s.instance.lastError && (
                  <div className="streams-row-error" title={s.instance.lastError}>
                    {s.instance.lastError.slice(0, 80)}
                  </div>
                )}
              </button>
            ))}
          </div>
        </div>

        <div className="card streams-tail">
          <div className="card-header">
            <h3>
              <Activity size={16} />
              Live record tail{selected ? ` — ${selected}` : ''}
            </h3>
            <span className="badge badge-success">{stream.recordsPerSec.toFixed(1)}/s</span>
          </div>
          <div className="streams-tail-list">
            {filtered.length === 0 && (
              <p className="empty-state">Waiting for records…</p>
            )}
            {filtered.slice(0, 50).map((r, i) => (
              <RecordRow key={i} record={r} />
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}

function RecordRow({ record }: { record: SunnyRecord }) {
  const payload = record.payload as globalThis.Record<string, unknown>;
  const headline =
    (payload?.headline as string) ||
    (payload?.event as string) ||
    (payload?.place as string) ||
    (payload?.siteName as string) ||
    (payload?.parameter as string) ||
    record.sourceId ||
    record.connectorId;
  return (
    <div className="stream-record">
      <div className="stream-record-head">
        <code className="stream-record-conn">{record.connectorId}</code>
        <span className="stream-record-time">{formatTimeAgo(record.timestamp)}</span>
      </div>
      <div className="stream-record-body">{headline}</div>
      <div className="stream-record-tags">
        {Object.entries(record.tags ?? {}).slice(0, 4).map(([k, v]) => (
          <code key={k}>
            {k}={v}
          </code>
        ))}
      </div>
    </div>
  );
}
