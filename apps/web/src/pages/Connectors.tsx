import { useEffect, useMemo, useState, type ReactElement } from 'react';
import {
  Plug,
  CheckCircle2,
  AlertTriangle,
  PauseCircle,
  Loader2,
  X,
  CircleDot,
  Settings,
  Link2,
  KeyRound,
  Copy,
  Globe,
} from 'lucide-react';
import { Sparkline } from '../components/charts/Sparkline';
import { getConnectors, getConnectorRegistry, getTimeseries } from '../api/sunny';
import type {
  Category,
  ConnectorsResponse,
  InstanceState,
  InstanceStatus,
  Manifest,
  RegistryDocument,
  RegistryEntry,
} from '../api/types';
import { formatTimeAgo } from '../utils/format';
import './Connectors.css';

const CATEGORY_LABEL: globalThis.Record<Category, string> = {
  geophysical: 'Geophysical',
  weather: 'Weather',
  air_quality: 'Air Quality',
  hydrology: 'Hydrology',
  wildfire: 'Wildfire',
  structural: 'Structural',
  iot: 'IoT',
  industrial: 'Industrial',
  custom: 'Custom',
};

const STATE_ICON: globalThis.Record<InstanceState, ReactElement> = {
  starting: <Loader2 size={12} className="spin" />,
  running: <CheckCircle2 size={12} />,
  backoff: <AlertTriangle size={12} />,
  stopped: <PauseCircle size={12} />,
  failed: <AlertTriangle size={12} />,
};

export default function Connectors() {
  const [conns, setConns] = useState<ConnectorsResponse | null>(null);
  const [registry, setRegistry] = useState<RegistryDocument | null>(null);
  const [open, setOpen] = useState<Manifest | null>(null);

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      try {
        const [c, r] = await Promise.all([
          getConnectors(),
          getConnectorRegistry().catch(() => null),
        ]);
        if (cancelled) return;
        setConns(c);
        if (r) setRegistry(r);
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

  const registryByID = useMemo(() => {
    const m = new Map<string, RegistryEntry>();
    for (const e of registry?.connectors ?? []) m.set(e.id, e);
    return m;
  }, [registry]);

  const grouped = useMemo(() => {
    if (!conns) return new Map<Category, Manifest[]>();
    const m = new Map<Category, Manifest[]>();
    for (const t of conns.types) {
      const arr = m.get(t.category) ?? [];
      arr.push(t);
      m.set(t.category, arr);
    }
    return m;
  }, [conns]);

  return (
    <div className="connectors-page">
      <div className="page-header">
        <div>
          <h1>Connectors</h1>
          <p className="page-subtitle">
            Plugin types compiled into this server, and the instances currently
            running. Each connector contract is the same: pull / push / stream.
          </p>
        </div>
      </div>

      <section className="card">
        <div className="card-header">
          <h3>
            <CircleDot size={16} />
            Running instances ({conns?.instances.length ?? 0})
          </h3>
        </div>
        <div className="connectors-instances">
          {!conns?.instances.length && (
            <p className="empty-state">No instances running. Add one in sunny.config.yaml and restart.</p>
          )}
          {conns?.instances.map((inst) => (
            <InstanceCard
              key={inst.instanceId}
              inst={inst}
              manifest={conns.types.find((t) => t.id === inst.type)}
            />
          ))}
        </div>
      </section>

      <section className="card">
        <div className="card-header">
          <h3>
            <Plug size={16} />
            Available connector types ({conns?.types.length ?? 0})
          </h3>
        </div>
        <div className="connector-marketplace">
          {[...grouped.entries()].map(([cat, types]) => (
            <div key={cat} className="marketplace-group">
              <h4 className="marketplace-group-label">{CATEGORY_LABEL[cat] ?? cat}</h4>
              <div className="marketplace-tiles">
                {types.map((t) => {
                  const running = conns!.instances.filter((i) => i.type === t.id).length;
                  return (
                    <button
                      key={t.id}
                      className="marketplace-tile"
                      onClick={() => setOpen(t)}
                    >
                      <div className="tile-head">
                        <span className="tile-name">{t.name}</span>
                        <span className="tile-mode">{t.mode}</span>
                      </div>
                      <p className="tile-desc">{t.description}</p>
                      <div className="tile-foot">
                        <code>{t.id}</code>
                        <span>v{t.version}</span>
                        {running > 0 && <span className="tile-running">{running} running</span>}
                      </div>
                    </button>
                  );
                })}
              </div>
            </div>
          ))}
        </div>
      </section>

      {open && (
        <ManifestModal
          manifest={open}
          registry={registryByID.get(open.id)}
          onClose={() => setOpen(null)}
        />
      )}
    </div>
  );
}

function InstanceCard({ inst, manifest }: { inst: InstanceStatus; manifest?: Manifest }) {
  const isPush = manifest?.mode === 'push';
  const ingestPath = `/api/ingest/${encodeURIComponent(inst.instanceId)}/`;
  const [series, setSeries] = useState<number[]>([]);

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      try {
        const buckets = await getTimeseries({
          connector: inst.instanceId,
          bucketSeconds: 60,
        });
        if (!cancelled) {
          setSeries(buckets.map((b) => b.count));
        }
      } catch {
        // ignore
      }
    };
    load();
    const id = window.setInterval(load, 30_000);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, [inst.instanceId]);

  return (
    <div className={`instance-card state-${inst.state}`}>
      <div className="instance-card-head">
        <strong>{inst.instanceId}</strong>
        <span className={`badge state-${inst.state}`}>
          {STATE_ICON[inst.state]} {inst.state}
        </span>
      </div>
      <div className="instance-card-meta">
        <code>{inst.type}</code>
        <span>started {formatTimeAgo(inst.startedAt)}</span>
        {inst.restarts > 0 && <span className="warn">restarts: {inst.restarts}</span>}
      </div>
      <div className="instance-card-spark" title="Records per minute, last hour">
        <Sparkline
          values={series}
          width={140}
          height={28}
          color="var(--accent)"
          strokeWidth={1.5}
        />
        <span className="instance-card-spark-label">last hour</span>
      </div>
      {isPush && (
        <div className="instance-card-push">
          <Link2 size={11} />
          <span>POST records to:</span>
          <code>{ingestPath}</code>
          <button
            className="copy-btn"
            onClick={() => navigator.clipboard?.writeText(window.location.origin + ingestPath)}
            title="Copy URL"
          >
            <Copy size={10} />
          </button>
        </div>
      )}
      {inst.lastError && (
        <div className="instance-card-error">{inst.lastError}</div>
      )}
    </div>
  );
}

function ManifestModal({
  manifest,
  registry,
  onClose,
}: {
  manifest: Manifest;
  registry?: RegistryEntry;
  onClose: () => void;
}) {
  const exampleInstanceID = `my-${manifest.id}`;
  const ingestURL = `${window.location.origin}/api/ingest/${exampleInstanceID}/`;
  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <header className="modal-head">
          <div>
            <h2>{manifest.name}</h2>
            <p className="modal-subtitle">
              <code>{manifest.id}</code> · v{manifest.version} · {manifest.mode}
              {registry?.verified && <span className="verified-badge">verified</span>}
              {registry?.homepage && (
                <a href={registry.homepage} target="_blank" rel="noreferrer" className="modal-link">
                  <Globe size={11} /> homepage
                </a>
              )}
            </p>
          </div>
          <button className="icon-btn" onClick={onClose} aria-label="Close">
            <X size={18} />
          </button>
        </header>

        <p className="modal-desc">{manifest.description}</p>

        {registry?.secrets && registry.secrets.length > 0 && (
          <section>
            <h3>
              <KeyRound size={14} />
              Required secrets
            </h3>
            <ul className="modal-secrets">
              {registry.secrets.map((s) => (
                <li key={s.name}>
                  <code>{s.envExample ?? s.name}</code>
                  {s.obtainUrl && (
                    <a href={s.obtainUrl} target="_blank" rel="noreferrer">
                      get one
                    </a>
                  )}
                </li>
              ))}
            </ul>
          </section>
        )}

        <section>
          <h3>
            <Settings size={14} />
            Configuration schema
          </h3>
          <SchemaPreview schema={manifest.configSchema} />
        </section>

        {manifest.mode === 'push' ? (
          <section>
            <h3>How to install (push mode)</h3>
            <p className="modal-help">
              1. Add an instance to <code>sunny.config.yaml</code>:
            </p>
            <pre>
{`connectors:
  - id: ${exampleInstanceID}
    type: ${manifest.id}
    config:
      requireToken: "your-token-here"`}
            </pre>
            <p className="modal-help">2. Send records via HTTP:</p>
            <pre>
{`curl -X POST ${ingestURL} \\
  -H "X-Sunny-Token: your-token-here" \\
  -H "X-Sunny-Source-Id: sensor-42" \\
  -H "X-Sunny-Tag-Severity: warning" \\
  -H "X-Sunny-Lat: 37.87" \\
  -H "X-Sunny-Lng: -122.27" \\
  -H "Content-Type: application/json" \\
  -d '{"temperature": 91.4}'`}
            </pre>
          </section>
        ) : (
          <section>
            <h3>How to install</h3>
            <p className="modal-help">
              Add to <code>sunny.config.yaml</code> and restart:
            </p>
            <pre>
{`connectors:
  - id: ${exampleInstanceID}
    type: ${manifest.id}
    config: { }`}
            </pre>
            <p className="modal-help">
              UI-driven install lands in v1.1.
            </p>
          </section>
        )}
      </div>
    </div>
  );
}

interface JsonSchemaProperty {
  type?: string;
  default?: unknown;
  description?: string;
  minimum?: number;
  enum?: unknown[];
}

interface JsonSchema {
  type?: string;
  properties?: globalThis.Record<string, JsonSchemaProperty>;
}

function SchemaPreview({ schema }: { schema: unknown }) {
  const s = (schema ?? {}) as JsonSchema;
  if (s.type !== 'object' || !s.properties) {
    return (
      <pre>{JSON.stringify(schema, null, 2)}</pre>
    );
  }
  const props = Object.entries(s.properties);
  if (props.length === 0) {
    return <p className="modal-help">No configuration required — defaults apply.</p>;
  }
  return (
    <table className="schema-table">
      <thead>
        <tr>
          <th>Field</th>
          <th>Type</th>
          <th>Default</th>
          <th>Description</th>
        </tr>
      </thead>
      <tbody>
        {props.map(([key, prop]) => (
          <tr key={key}>
            <td>
              <code>{key}</code>
            </td>
            <td>{prop.type ?? '—'}</td>
            <td>
              {prop.default !== undefined ? (
                <code>{JSON.stringify(prop.default)}</code>
              ) : (
                '—'
              )}
            </td>
            <td>{prop.description ?? ''}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
