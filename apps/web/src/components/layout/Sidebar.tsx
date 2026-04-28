import { NavLink } from 'react-router-dom';
import {
  LayoutDashboard,
  Map,
  Radio,
  AlertTriangle,
  Plug,
} from 'lucide-react';
import './Sidebar.css';

const NAV_ITEMS = [
  { to: '/', icon: LayoutDashboard, label: 'Dashboard', shortcut: '1' },
  { to: '/map', icon: Map, label: 'Live Map', shortcut: '2' },
  { to: '/streams', icon: Radio, label: 'Data Streams', shortcut: '3' },
  { to: '/alerts', icon: AlertTriangle, label: 'Alerts', shortcut: '4' },
  { to: '/connectors', icon: Plug, label: 'Connectors', shortcut: '5' },
];

export default function Sidebar() {
  return (
    <aside className="sidebar" aria-label="Primary navigation">
      <nav className="sidebar-nav">
        {NAV_ITEMS.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            end={item.to === '/'}
            title={item.label}
            className={({ isActive }) =>
              `sidebar-link ${isActive ? 'sidebar-link-active' : ''}`
            }
          >
            <item.icon size={16} strokeWidth={1.75} />
            <span className="sidebar-link-label">{item.label}</span>
          </NavLink>
        ))}
      </nav>
    </aside>
  );
}
