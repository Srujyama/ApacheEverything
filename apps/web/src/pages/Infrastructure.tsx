import { useState } from 'react';
import {
  Building2, Activity, AlertTriangle, MapPin, Calendar, Radio,
  ChevronDown, ChevronUp, Shield,
} from 'lucide-react';
import MetricCard from '../components/common/MetricCard';
import { getAssets, getSensors, getSensorsByStatus } from '../services/sensorService';
import { conditionColor } from '../utils/format';
import './Infrastructure.css';

export default function Infrastructure() {
  const [expandedAsset, setExpandedAsset] = useState<string | null>(null);
  const [filterCategory, setFilterCategory] = useState<string>('all');

  const assets = getAssets();
  const sensors = getSensors();
  const sensorStatus = getSensorsByStatus();

  const filteredAssets = filterCategory === 'all'
    ? assets
    : assets.filter(a => a.category === filterCategory);

  const categories = [...new Set(assets.map(a => a.category))];

  const avgRisk = Math.round(assets.reduce((s, a) => s + a.riskScore, 0) / assets.length);
  const criticalAssets = assets.filter(a => a.riskScore >= 60).length;

  return (
    <div className="infra-page">
      <div className="page-header">
        <div>
          <h1>Infrastructure Assets</h1>
          <p className="page-subtitle">
            Monitored physical infrastructure with sensor coverage and risk assessment
          </p>
        </div>
      </div>

      <div className="metrics-grid">
        <MetricCard label="Total Assets" value={assets.length} icon={<Building2 size={16} />} accentColor="var(--accent)" />
        <MetricCard label="Total Sensors" value={sensors.length} subtitle={`${sensorStatus.online} online, ${sensorStatus.offline} offline`} icon={<Activity size={16} />} accentColor="var(--status-good)" />
        <MetricCard label="Avg Risk Score" value={avgRisk} subtitle="out of 100" icon={<Shield size={16} />} accentColor={avgRisk > 40 ? 'var(--severity-warning)' : 'var(--status-good)'} />
        <MetricCard label="Critical Risk" value={criticalAssets} subtitle="assets above 60 risk score" icon={<AlertTriangle size={16} />} accentColor="var(--severity-critical)" />
      </div>

      {/* Category Filter */}
      <div className="infra-filters">
        <button
          className={`infra-filter-btn ${filterCategory === 'all' ? 'active' : ''}`}
          onClick={() => setFilterCategory('all')}
        >
          All ({assets.length})
        </button>
        {categories.map(cat => (
          <button
            key={cat}
            className={`infra-filter-btn ${filterCategory === cat ? 'active' : ''}`}
            onClick={() => setFilterCategory(cat)}
          >
            {cat.replace(/_/g, ' ')} ({assets.filter(a => a.category === cat).length})
          </button>
        ))}
      </div>

      {/* Asset List */}
      <div className="asset-list">
        {filteredAssets
          .sort((a, b) => b.riskScore - a.riskScore)
          .map(asset => {
            const assetSensors = sensors.filter(s => asset.sensors.includes(s.id));
            const isExpanded = expandedAsset === asset.id;

            return (
              <div
                key={asset.id}
                className={`asset-card ${isExpanded ? 'expanded' : ''}`}
                onClick={() => setExpandedAsset(isExpanded ? null : asset.id)}
              >
                <div className="asset-card-main">
                  <div className="asset-risk-bar">
                    <div
                      className="asset-risk-fill"
                      style={{
                        height: `${asset.riskScore}%`,
                        background: asset.riskScore >= 60 ? 'var(--severity-critical)'
                          : asset.riskScore >= 40 ? 'var(--severity-warning)'
                          : 'var(--status-good)',
                      }}
                    />
                  </div>
                  <div className="asset-card-content">
                    <div className="asset-card-header">
                      <div>
                        <h3>{asset.name}</h3>
                        <div className="asset-card-meta">
                          <span className="asset-category">{asset.category.replace(/_/g, ' ')}</span>
                          <span style={{ color: conditionColor(asset.condition) }} className="asset-condition">
                            {asset.condition}
                          </span>
                        </div>
                      </div>
                      <div className="asset-risk-display">
                        <span className="asset-risk-value" style={{
                          color: asset.riskScore >= 60 ? 'var(--severity-critical)'
                            : asset.riskScore >= 40 ? 'var(--severity-warning)'
                            : 'var(--status-good)'
                        }}>
                          {asset.riskScore}
                        </span>
                        <span className="asset-risk-label">Risk Score</span>
                      </div>
                    </div>

                    <div className="asset-quick-info">
                      <span><MapPin size={12} /> {asset.location.lat.toFixed(2)}, {asset.location.lng.toFixed(2)}</span>
                      <span><Radio size={12} /> {asset.sensors.length} sensors</span>
                      <span><Calendar size={12} /> Last inspected: {asset.lastInspection}</span>
                      <span><Calendar size={12} /> Next: {asset.nextInspection}</span>
                    </div>

                    {isExpanded && (
                      <div className="asset-expanded">
                        {/* Metadata */}
                        <div className="asset-metadata">
                          <h4>Asset Details</h4>
                          <div className="metadata-grid">
                            {Object.entries(asset.metadata).map(([key, val]) => (
                              <div key={key} className="metadata-item">
                                <span className="metadata-key">{key.replace(/_/g, ' ')}</span>
                                <span className="metadata-value">{val}</span>
                              </div>
                            ))}
                          </div>
                        </div>

                        {/* Sensors */}
                        {assetSensors.length > 0 && (
                          <div className="asset-sensors">
                            <h4>Connected Sensors</h4>
                            <div className="sensor-grid">
                              {assetSensors.map(s => (
                                <div key={s.id} className="sensor-chip">
                                  <span className={`status-dot ${s.status}`} />
                                  <div className="sensor-chip-info">
                                    <span className="sensor-chip-name">{s.name}</span>
                                    <span className="sensor-chip-reading">{s.lastReading} {s.unit}</span>
                                  </div>
                                </div>
                              ))}
                            </div>
                          </div>
                        )}
                      </div>
                    )}
                  </div>
                  <div className="asset-expand-icon">
                    {isExpanded ? <ChevronUp size={16} /> : <ChevronDown size={16} />}
                  </div>
                </div>
              </div>
            );
          })}
      </div>
    </div>
  );
}
