import { BrowserRouter, Routes, Route } from 'react-router-dom';
import Layout from './components/layout/Layout';
import Dashboard from './pages/Dashboard';
import LiveMap from './pages/LiveMap';
import DataStreams from './pages/DataStreams';
import Alerts from './pages/Alerts';
import Infrastructure from './pages/Infrastructure';
import Analytics from './pages/Analytics';
import DataSources from './pages/DataSources';

export default function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<Layout />}>
          <Route path="/" element={<Dashboard />} />
          <Route path="/map" element={<LiveMap />} />
          <Route path="/streams" element={<DataStreams />} />
          <Route path="/alerts" element={<Alerts />} />
          <Route path="/assets" element={<Infrastructure />} />
          <Route path="/analytics" element={<Analytics />} />
          <Route path="/settings" element={<DataSources />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}
