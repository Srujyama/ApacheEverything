import { NavLink } from 'react-router-dom';
import {
  LayoutDashboard,
  Map,
  Radio,
  AlertTriangle,
  Plug,
  Eye,
  Activity,
} from 'lucide-react';
import { useEffect, useState } from 'react';
import { getConnectorInstances, getHealth } from '../../api/sunny';
import './Sidebar.css';

const NAV_ITEMS = [
  { to: '/', icon: LayoutDashboard, label: 'Dashboard' },
  { to: '/map', icon: Map, label: 'Live Map' },
  { to: '/streams', icon: Radio, label: 'Data Streams' },
  { to: '/alerts', icon: AlertTriangle, label: 'Alerts' },
  { to: '/connectors', icon: Plug, label: 'Connectors' },
];

export default function Sidebar() {
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

  return (
    <aside className="sidebar">
      <div className="sidebar-header">
        <div className="sidebar-logo">
          <Eye size={24} />
          <div className="sidebar-logo-text">
            <span className="sidebar-title">Sunny</span>
            <span className="sidebar-subtitle">Physical Observability</span>
          </div>
        </div>
      </div>

      <div className="sidebar-status">
        <div className="status-indicator">
          <Activity size={14} className={healthy ? 'pulse' : ''} />
          <span>{healthy ? 'Server healthy' : 'Server unreachable'}</span>
        </div>
      </div>

      <nav className="sidebar-nav">
        {NAV_ITEMS.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.to === '/'}
            className={({ isActive }) =>
              `sidebar-link ${isActive ? 'sidebar-link-active' : ''}`
            }
          >
            <item.icon size={18} />
            <span>{item.label}</span>
          </NavLink>
        ))}
      </nav>

      <div className="sidebar-footer">
        <div className="sidebar-footer-item">
          <span className={`status-dot ${healthy ? 'online pulse-dot' : 'offline'}`} />
          <span>{running ?? '…'} connectors running</span>
        </div>
      </div>
    </aside>
  );
}
