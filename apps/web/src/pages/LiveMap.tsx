import { useEffect, useMemo, useState } from 'react';
import { MapContainer, TileLayer, CircleMarker, Popup } from 'react-leaflet';
import 'leaflet/dist/leaflet.css';
import { useLiveStream } from '../hooks/useLiveStream';
import { getRecords, getConnectors } from '../api/sunny';
import type { ConnectorsResponse, Record as SunnyRecord } from '../api/types';
import { formatTimeAgo } from '../utils/format';
import './LiveMap.css';

const SEVERITY_FILL: globalThis.Record<string, string> = {
  emergency: '#dc2626',
  critical: '#ef4444',
  warning: '#f59e0b',
  info: '#3b82f6',
};

function colorForRecord(r: SunnyRecord): string {
  const sev = (r.tags?.severity ?? '').toLowerCase();
  if (sev in SEVERITY_FILL) return SEVERITY_FILL[sev];
  return '#10b981';
}

export default function LiveMap() {
  const [historical, setHistorical] = useState<SunnyRecord[]>([]);
  const [conns, setConns] = useState<ConnectorsResponse | null>(null);
  const [enabledConns, setEnabledConns] = useState<Set<string>>(new Set());
  const stream = useLiveStream({ bufferSize: 1000, replay: false });

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      try {
        const [c, rs] = await Promise.all([
          getConnectors(),
          getRecords({ limit: 1000 }),
        ]);
        if (cancelled) return;
        setConns(c);
        setHistorical(rs);
        // First load: enable all connectors that produce locations.
        if (enabledConns.size === 0) {
          const has = new Set<string>();
          for (const r of rs) if (r.location) has.add(r.connectorId);
          setEnabledConns(has);
        }
      } catch {
        // ignore
      }
    };
    load();
    const id = window.setInterval(load, 30000);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const points = useMemo(() => {
    const merged = [...stream.records, ...historical];
    const seen = new Set<string>();
    const out: SunnyRecord[] = [];
    for (const r of merged) {
      if (!r.location) continue;
      if (!enabledConns.has(r.connectorId)) continue;
      const key = `${r.connectorId}|${r.sourceId ?? ''}|${r.timestamp}`;
      if (seen.has(key)) continue;
      seen.add(key);
      out.push(r);
      if (out.length >= 500) break; // keep map snappy
    }
    return out;
  }, [stream.records, historical, enabledConns]);

  const togglable = useMemo(() => {
    const has = new Set<string>();
    for (const r of [...stream.records, ...historical]) {
      if (r.location) has.add(r.connectorId);
    }
    return [...has];
  }, [stream.records, historical]);

  const toggle = (id: string) => {
    setEnabledConns((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  return (
    <div className="livemap-page">
      <div className="page-header">
        <div>
          <h1>Live Map</h1>
          <p className="page-subtitle">
            Records with geographic coordinates plotted in real time. Toggle
            individual connector instances on the right.
          </p>
        </div>
        <div className="live-indicator">
          <span className={`status-dot ${stream.connected ? 'online pulse-dot' : 'offline'}`} />
          <span>{stream.connected ? 'live' : 'offline'}</span>
        </div>
      </div>

      <div className="livemap-grid">
        <div className="livemap-canvas card">
          <MapContainer
            center={[39.8283, -98.5795]}
            zoom={4}
            scrollWheelZoom
            style={{ width: '100%', height: '100%', borderRadius: 'var(--radius-md)' }}
          >
            <TileLayer
              attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>'
              url="https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"
            />
            {points.map((r, i) => (
              <CircleMarker
                key={i}
                center={[r.location!.lat, r.location!.lng]}
                radius={5}
                pathOptions={{
                  color: colorForRecord(r),
                  fillColor: colorForRecord(r),
                  fillOpacity: 0.7,
                  weight: 1,
                }}
              >
                <Popup>
                  <RecordPopup record={r} />
                </Popup>
              </CircleMarker>
            ))}
          </MapContainer>
        </div>

        <div className="livemap-sidebar card">
          <div className="card-header">
            <h3>Layers</h3>
          </div>
          <div className="livemap-layers">
            {togglable.length === 0 && (
              <p className="empty-state">No location-bearing records yet.</p>
            )}
            {togglable.map((id) => {
              const inst = conns?.instances.find((i) => i.instanceId === id);
              const type = inst ? conns?.types.find((t) => t.id === inst.type) : null;
              return (
                <label key={id} className="livemap-layer">
                  <input
                    type="checkbox"
                    checked={enabledConns.has(id)}
                    onChange={() => toggle(id)}
                  />
                  <span>
                    <strong>{id}</strong>
                    <small>{type?.name ?? inst?.type}</small>
                  </span>
                </label>
              );
            })}
          </div>
          <div className="livemap-legend">
            {Object.entries(SEVERITY_FILL).map(([k, v]) => (
              <div key={k} className="livemap-legend-item">
                <span className="dot" style={{ background: v }} />
                <span>{k}</span>
              </div>
            ))}
            <div className="livemap-legend-item">
              <span className="dot" style={{ background: '#10b981' }} />
              <span>(no severity)</span>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}

function RecordPopup({ record }: { record: SunnyRecord }) {
  const payload = record.payload as globalThis.Record<string, unknown>;
  const headline =
    (payload?.headline as string) ||
    (payload?.event as string) ||
    (payload?.place as string) ||
    (payload?.siteName as string) ||
    record.connectorId;
  return (
    <div className="livemap-popup">
      <strong>{headline}</strong>
      <div className="livemap-popup-meta">
        <code>{record.connectorId}</code>
        <span>{formatTimeAgo(record.timestamp)}</span>
      </div>
      <pre>{JSON.stringify(payload, null, 2).slice(0, 500)}</pre>
    </div>
  );
}
