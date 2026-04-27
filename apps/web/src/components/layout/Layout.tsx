import { Outlet } from 'react-router-dom';
import { useEffect, useState } from 'react';
import { Menu, X } from 'lucide-react';
import Sidebar from './Sidebar';
import './Layout.css';

export default function Layout() {
  const [open, setOpen] = useState(false);

  // Close the mobile drawer whenever the viewport returns to desktop width.
  useEffect(() => {
    const onResize = () => {
      if (window.innerWidth >= 900 && open) setOpen(false);
    };
    window.addEventListener('resize', onResize);
    return () => window.removeEventListener('resize', onResize);
  }, [open]);

  // Lock scroll when the drawer is open on mobile.
  useEffect(() => {
    document.body.style.overflow = open ? 'hidden' : '';
    return () => {
      document.body.style.overflow = '';
    };
  }, [open]);

  return (
    <div className={`layout ${open ? 'layout-drawer-open' : ''}`}>
      <button
        className="mobile-toggle"
        aria-label={open ? 'Close menu' : 'Open menu'}
        onClick={() => setOpen((v) => !v)}
      >
        {open ? <X size={20} /> : <Menu size={20} />}
      </button>

      <Sidebar />
      {open && <div className="sidebar-backdrop" onClick={() => setOpen(false)} />}

      <main className="main-content" onClick={() => open && setOpen(false)}>
        <Outlet />
      </main>
    </div>
  );
}
