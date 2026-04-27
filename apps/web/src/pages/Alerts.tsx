import { useEffect, useMemo, useState } from 'react';
import { AlertTriangle, Filter, Search, Check } from 'lucide-react';
import { ackAlert, getAlertRules, getAlerts } from '../api/sunny';
import type { Alert, AlertRule } from '../api/types';
import { formatTimeAgo } from '../utils/format';
import './Alerts.css';

const SEVERITIES = ['emergency', 'critical', 'warning', 'info'] as const;
type Severity = (typeof SEVERITIES)[number];

const SEVERITY_FILL: globalThis.Record<Severity, string> = {
  emergency: 'var(--severity-emergency)',
  critical: 'var(--severity-critical)',
  warning: 'var(--severity-warning)',
  info: 'var(--severity-info)',
};

export default function Alerts() {
  const [alerts, setAlerts] = useState<Alert[]>([]);
  const [rules, setRules] = useState<AlertRule[]>([]);
  const [filter, setFilter] = useState<Set<Severity>>(new Set(SEVERITIES));
  const [showAcked, setShowAcked] = useState(false);
  const [query, setQuery] = useState('');
  const [error, setError] = useState<string | null>(null);

  const refresh = useMemo(
    () => async () => {
      try {
        const [a, r] = await Promise.all([getAlerts(200), getAlertRules()]);
        setAlerts(a);
        setRules(r);
        setError(null);
      } catch (e) {
        setError(e instanceof Error ? e.message : String(e));
      }
    },
    [],
  );

  useEffect(() => {
    refresh();
    const id = window.setInterval(refresh, 5000);
    return () => window.clearInterval(id);
  }, [refresh]);

  const filtered = useMemo(() => {
    return alerts.filter((a) => {
      const sev = (a.severity || '').toLowerCase();
      if (sev && !filter.has(sev as Severity)) return false;
      if (!showAcked && a.acked) return false;
      if (query) {
        const blob = JSON.stringify(a).toLowerCase();
        if (!blob.includes(query.toLowerCase())) return false;
      }
      return true;
    });
  }, [alerts, filter, showAcked, query]);

  const counts = useMemo(() => {
    const c: globalThis.Record<Severity, number> = {
      emergency: 0,
      critical: 0,
      warning: 0,
      info: 0,
    };
    for (const a of alerts) {
      const s = (a.severity || '').toLowerCase();
      if (s in c) c[s as Severity]++;
    }
    return c;
  }, [alerts]);

  const toggle = (s: Severity) => {
    setFilter((prev) => {
      const next = new Set(prev);
      if (next.has(s)) next.delete(s);
      else next.add(s);
      return next;
    });
  };

  const handleAck = async (id: string) => {
    try {
      await ackAlert(id);
      refresh();
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  };

  return (
    <div className="alerts-page">
      <div className="page-header">
        <div>
          <h1>Alerts</h1>
          <p className="page-subtitle">
            Triggered by the rule engine. {rules.length} rule
            {rules.length === 1 ? '' : 's'} active.
          </p>
        </div>
      </div>

      {error && (
        <div className="card alerts-error">
          <strong>API error:</strong> {error}
        </div>
      )}

      <div className="alerts-filters card">
        <div className="alerts-filter-group">
          <Filter size={14} />
          {SEVERITIES.map((s) => (
            <button
              key={s}
              className={`alerts-filter-chip ${filter.has(s) ? 'on' : ''}`}
              style={filter.has(s) ? { borderColor: SEVERITY_FILL[s] } : undefined}
              onClick={() => toggle(s)}
            >
              <span className="dot" style={{ background: SEVERITY_FILL[s] }} />
              {s} <small>({counts[s]})</small>
            </button>
          ))}
          <button
            className={`alerts-filter-chip ${showAcked ? 'on' : ''}`}
            onClick={() => setShowAcked((v) => !v)}
          >
            show acked
          </button>
        </div>
        <div className="alerts-search">
          <Search size={14} />
          <input
            placeholder="Search alerts…"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
        </div>
      </div>

      <div className="alerts-list">
        {filtered.length === 0 && (
          <div className="card empty-state">
            No alerts match. Either nothing critical is happening (good), or no
            rules are configured for the data flowing in. Default rule fires on
            <code>tags.severity</code> in <code>[emergency, critical]</code>.
          </div>
        )}
        {filtered.map((a) => {
          const sev = (a.severity || 'info').toLowerCase() as Severity;
          const fill = SEVERITY_FILL[sev] ?? SEVERITY_FILL.info;
          const payload = (a.payload ?? {}) as globalThis.Record<string, unknown>;
          const description =
            (payload.description as string) ||
            (payload.areaDesc as string) ||
            '';
          return (
            <div key={a.id} className={`alert-row ${a.acked ? 'alert-row-acked' : ''}`}>
              <div className="alert-row-side" style={{ background: fill }} />
              <div className="alert-row-body">
                <div className="alert-row-head">
                  <span
                    className="badge"
                    style={{ background: fill, color: 'white' }}
                  >
                    {sev || 'unknown'}
                  </span>
                  <code>{a.connectorId}</code>
                  {a.sourceId && <code className="muted">{a.sourceId}</code>}
                  <code className="muted">rule: {a.ruleName}</code>
                  <span className="alert-row-time">{formatTimeAgo(a.triggered)}</span>
                </div>
                <p className="alert-row-headline">
                  <AlertTriangle size={14} /> {a.headline || '(no headline)'}
                </p>
                {description && (
                  <p className="alert-row-desc">
                    {description.slice(0, 280)}
                    {description.length > 280 ? '…' : ''}
                  </p>
                )}
                <div className="alert-row-tags">
                  {Object.entries(a.tags ?? {}).slice(0, 6).map(([k, v]) => (
                    <code key={k}>{k}={v}</code>
                  ))}
                </div>
                {!a.acked && (
                  <button className="alert-ack-btn" onClick={() => handleAck(a.id)}>
                    <Check size={12} /> Acknowledge
                  </button>
                )}
                {a.acked && (
                  <span className="alert-acked-tag">acknowledged {formatTimeAgo(a.acked)}</span>
                )}
              </div>
            </div>
          );
        })}
      </div>
    </div>
  );
}
