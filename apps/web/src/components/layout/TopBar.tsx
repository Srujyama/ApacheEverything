import { useEffect, useState } from 'react';
import { useLocation } from 'react-router-dom';
import { Activity } from 'lucide-react';
import { getConnectorInstances, getHealth } from '../../api/sunny';

const PAGE_TITLES: Record<string, string> = {
  '/': 'Dashboard',
  '/map': 'Live Map',
  '/streams': 'Data Streams',
  '/alerts': 'Alerts',
  '/connectors': 'Connectors',
};

export default function TopBar() {
  const location = useLocation();
  const [running, setRunning] = useState<number | null>(null);
  const [healthy, setHealthy] = useState(false);

  useEffect(() => {
    let cancelled = false;
    const tick = async () => {
      try {
        const [insts, h] = await Promise.all([getConnectorInstances(), getHealth()]);
        if (cancelled) return;
        setRunning(insts.filter((i) => i.state === 'running').length);
        setHealthy(h.status === 'ok');
      } catch {
        if (!cancelled) setHealthy(false);
      }
    };
    tick();
    const id = window.setInterval(tick, 5000);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, []);

  const pageTitle = PAGE_TITLES[location.pathname] ?? 'Sunny';

  return (
    <header className="topbar">
      <div className="topbar-brand">
        <span className="topbar-mark">SUNNY</span>
        <span className="topbar-sep">/</span>
        <span className="topbar-context">{pageTitle}</span>
      </div>

      <div className="topbar-meta">
        <a href="/api/docs" className="topbar-link" target="_blank" rel="noreferrer">
          API
        </a>
        <span className="topbar-divider" aria-hidden />
        <span className="topbar-stat">
          <span className="topbar-stat-label">RUN</span>
          <span className="topbar-stat-value">{running ?? '—'}</span>
        </span>
        <span className="topbar-divider" aria-hidden />
        <span className={`topbar-health ${healthy ? 'ok' : 'down'}`}>
          <Activity size={11} strokeWidth={2.25} />
          <span>{healthy ? 'CONNECTED' : 'OFFLINE'}</span>
        </span>
      </div>
    </header>
  );
}
