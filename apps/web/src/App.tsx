import { BrowserRouter, Routes, Route, Navigate } from 'react-router-dom';
import { lazy, Suspense } from 'react';
import AuthGate from './components/AuthGate';
import Layout from './components/layout/Layout';

// Lazy-load every page so the initial bundle stays small. Recharts and
// react-leaflet are big and only needed once you visit those pages.
const Dashboard = lazy(() => import('./pages/Dashboard'));
const LiveMap = lazy(() => import('./pages/LiveMap'));
const DataStreams = lazy(() => import('./pages/DataStreams'));
const Alerts = lazy(() => import('./pages/Alerts'));
const Connectors = lazy(() => import('./pages/Connectors'));

function PageFallback() {
  return <div className="page-loading">Loading…</div>;
}

export default function App() {
  return (
    <AuthGate>
      <BrowserRouter>
        <Suspense fallback={<PageFallback />}>
          <Routes>
            <Route element={<Layout />}>
              <Route path="/" element={<Dashboard />} />
              <Route path="/map" element={<LiveMap />} />
              <Route path="/streams" element={<DataStreams />} />
              <Route path="/alerts" element={<Alerts />} />
              <Route path="/connectors" element={<Connectors />} />
              {/* Old routes from the mock era — redirect rather than 404. */}
              <Route path="/settings" element={<Navigate to="/connectors" replace />} />
              <Route path="/assets" element={<Navigate to="/" replace />} />
              <Route path="/analytics" element={<Navigate to="/streams" replace />} />
            </Route>
          </Routes>
        </Suspense>
      </BrowserRouter>
    </AuthGate>
  );
}
