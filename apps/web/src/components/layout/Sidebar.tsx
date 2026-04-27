import { NavLink } from 'react-router-dom';
import {
  LayoutDashboard,
  Map,
  Radio,
  AlertTriangle,
  Building2,
  TrendingUp,
  Settings,
  Activity,
  Eye,
  Zap,
} from 'lucide-react';
import './Sidebar.css';

const NAV_ITEMS = [
  { to: '/', icon: LayoutDashboard, label: 'Dashboard' },
  { to: '/map', icon: Map, label: 'Live Map' },
  { to: '/streams', icon: Radio, label: 'Data Streams' },
  { to: '/alerts', icon: AlertTriangle, label: 'Alerts' },
  { to: '/assets', icon: Building2, label: 'Infrastructure' },
  { to: '/analytics', icon: TrendingUp, label: 'Analytics' },
  { to: '/settings', icon: Settings, label: 'Data Sources' },
];

export default function Sidebar() {
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
          <Activity size={14} className="pulse" />
          <span>Platform Active</span>
        </div>
        <div className="pipeline-badge">
          <Zap size={12} />
          <span>Kafka + Spark</span>
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
          <span className="status-dot online pulse-dot" />
          <span>8 data sources connected</span>
        </div>
        <div className="sidebar-footer-item">
          <span className="status-dot online" />
          <span>24 sensors active</span>
        </div>
      </div>
    </aside>
  );
}
