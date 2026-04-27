import { useState, useEffect } from 'react';
import {
  AlertTriangle, Filter, Clock, MapPin, Cpu, Radio, Eye, Tag,
} from 'lucide-react';
import { getAlerts } from '../services/alertService';
import { fetchActiveWeatherAlerts } from '../services/publicDataSources';
import type { Alert, AlertSeverity } from '../types';
import { formatTimeAgo, formatTimestamp } from '../utils/format';
import MetricCard from '../components/common/MetricCard';
import './Alerts.css';

const SEVERITY_OPTIONS: (AlertSeverity | 'all')[] = ['all', 'emergency', 'critical', 'warning', 'info'];

export default function Alerts() {
  const [selectedSeverity, setSelectedSeverity] = useState<AlertSeverity | 'all'>('all');
  const [weatherAlerts, setWeatherAlerts] = useState<Alert[]>([]);
  const [expandedAlert, setExpandedAlert] = useState<string | null>(null);

  const internalAlerts = getAlerts();

  useEffect(() => {
    fetchActiveWeatherAlerts().then(setWeatherAlerts);
  }, []);

  const allAlerts = [...internalAlerts, ...weatherAlerts]
    .sort((a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime());

  const filteredAlerts = selectedSeverity === 'all'
    ? allAlerts
    : allAlerts.filter(a => a.severity === selectedSeverity);

  const counts = {
    emergency: allAlerts.filter(a => a.severity === 'emergency').length,
    critical: allAlerts.filter(a => a.severity === 'critical').length,
    warning: allAlerts.filter(a => a.severity === 'warning').length,
    info: allAlerts.filter(a => a.severity === 'info').length,
  };

  return (
    <div className="alerts-page">
      <div className="page-header">
        <div>
          <h1>Alerts</h1>
          <p className="page-subtitle">
            Real-time alerts from sensors, AI models, Spark analytics, and public APIs (NOAA, USGS)
          </p>
        </div>
      </div>

      <div className="metrics-grid">
        <MetricCard label="Emergency" value={counts.emergency} accentColor="var(--severity-emergency)" icon={<AlertTriangle size={16} />} />
        <MetricCard label="Critical" value={counts.critical} accentColor="var(--severity-critical)" icon={<AlertTriangle size={16} />} />
        <MetricCard label="Warning" value={counts.warning} accentColor="var(--severity-warning)" icon={<AlertTriangle size={16} />} />
        <MetricCard label="Info" value={counts.info} accentColor="var(--severity-info)" icon={<Eye size={16} />} />
      </div>

      <div className="alerts-toolbar">
        <div className="severity-filters">
          <Filter size={14} />
          {SEVERITY_OPTIONS.map(sev => (
            <button
              key={sev}
              className={`severity-filter-btn ${selectedSeverity === sev ? 'active' : ''} ${sev !== 'all' ? `filter-${sev}` : ''}`}
              onClick={() => setSelectedSeverity(sev)}
            >
              {sev === 'all' ? 'All' : sev}
              {sev !== 'all' && <span className="filter-count">{counts[sev]}</span>}
            </button>
          ))}
        </div>
        <span className="alert-total">{filteredAlerts.length} alerts</span>
      </div>

      <div className="alerts-list">
        {filteredAlerts.map(alert => (
          <div
            key={alert.id}
            className={`alert-card severity-${alert.severity} ${expandedAlert === alert.id ? 'expanded' : ''}`}
            onClick={() => setExpandedAlert(expandedAlert === alert.id ? null : alert.id)}
          >
            <div className="alert-card-indicator">
              <span className={`status-dot ${alert.severity === 'emergency' ? 'offline' : alert.severity === 'critical' ? 'degraded' : alert.severity === 'warning' ? 'degraded' : 'active'} ${alert.status === 'active' ? 'pulse-dot' : ''}`} />
            </div>
            <div className="alert-card-content">
              <div className="alert-card-top">
                <span className={`badge badge-${alert.severity}`}>{alert.severity}</span>
                <span className="alert-source-badge">
                  {alert.source === 'spark_analytics' && <Cpu size={10} />}
                  {alert.source === 'public_api' && <Radio size={10} />}
                  {alert.source === 'sensor' && <Eye size={10} />}
                  {alert.source === 'ai_model' && <Cpu size={10} />}
                  {alert.source}
                </span>
                <span className="alert-status-badge">{alert.status}</span>
                <span className="alert-time">
                  <Clock size={10} /> {formatTimeAgo(alert.timestamp)}
                </span>
              </div>
              <h3 className="alert-card-title">{alert.title}</h3>
              {expandedAlert === alert.id && (
                <div className="alert-card-expanded">
                  <p className="alert-description">{alert.description}</p>
                  <div className="alert-detail-grid">
                    {alert.location && (
                      <div className="alert-detail">
                        <MapPin size={12} />
                        <span>{alert.location.lat.toFixed(4)}, {alert.location.lng.toFixed(4)}</span>
                      </div>
                    )}
                    {alert.assetId && (
                      <div className="alert-detail">
                        <Eye size={12} />
                        <span>Asset: {alert.assetId}</span>
                      </div>
                    )}
                    {alert.sensorId && (
                      <div className="alert-detail">
                        <Radio size={12} />
                        <span>Sensor: {alert.sensorId}</span>
                      </div>
                    )}
                    <div className="alert-detail">
                      <Clock size={12} />
                      <span>{formatTimestamp(alert.timestamp)}</span>
                    </div>
                  </div>
                  <div className="alert-tags">
                    <Tag size={12} />
                    {alert.tags.map(tag => <code key={tag}>{tag}</code>)}
                  </div>
                </div>
              )}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
