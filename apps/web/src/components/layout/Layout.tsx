import { Outlet } from 'react-router-dom';
import { useEffect, useState } from 'react';
import { Menu, X } from 'lucide-react';
import Sidebar from './Sidebar';
import TopBar from './TopBar';
import './TopBar.css';
import './Layout.css';

export default function Layout() {
  const [open, setOpen] = useState(false);

  useEffect(() => {
    const onResize = () => {
      if (window.innerWidth >= 900 && open) setOpen(false);
    };
    window.addEventListener('resize', onResize);
    return () => window.removeEventListener('resize', onResize);
  }, [open]);

  useEffect(() => {
    document.body.style.overflow = open ? 'hidden' : '';
    return () => {
      document.body.style.overflow = '';
    };
  }, [open]);

  return (
    <div className={`layout ${open ? 'layout-drawer-open' : ''}`}>
      <TopBar />
      <div className="layout-body">
        <button
          className="mobile-toggle"
          aria-label={open ? 'Close menu' : 'Open menu'}
          onClick={() => setOpen((v) => !v)}
        >
          {open ? <X size={18} /> : <Menu size={18} />}
        </button>

        <Sidebar />
        {open && <div className="sidebar-backdrop" onClick={() => setOpen(false)} />}

        <main className="main-content" onClick={() => open && setOpen(false)}>
          <Outlet />
        </main>
      </div>
    </div>
  );
}
