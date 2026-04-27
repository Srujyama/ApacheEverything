import {
  Satellite, Globe, ExternalLink, RefreshCw, Database, Radio, Clock,
  AlertTriangle, CheckCircle,
} from 'lucide-react';
import MetricCard from '../components/common/MetricCard';
import { getAllDataSources } from '../services/publicDataSources';
import { formatNumber, formatTimeAgo } from '../utils/format';
import './DataSources.css';

export default function DataSources() {
  const sources = getAllDataSources();
  const connected = sources.filter(s => s.status === 'connected').length;
  const totalRecords = sources.reduce((s, d) => s + d.recordsIngested, 0);
  const avgErrorRate = sources.reduce((s, d) => s + d.errorRate, 0) / sources.length;

  return (
    <div className="data-sources-page">
      <div className="page-header">
        <div>
          <h1>Data Sources</h1>
          <p className="page-subtitle">
            Public infrastructure APIs feeding into the Kafka ingestion pipeline
          </p>
        </div>
      </div>

      <div className="metrics-grid">
        <MetricCard label="Connected Sources" value={connected} subtitle={`of ${sources.length} configured`} icon={<Satellite size={16} />} accentColor="var(--status-good)" />
        <MetricCard label="Total Records Ingested" value={formatNumber(totalRecords)} icon={<Database size={16} />} accentColor="var(--accent)" />
        <MetricCard label="Avg Error Rate" value={`${(avgErrorRate * 100).toFixed(2)}%`} icon={<AlertTriangle size={16} />} accentColor={avgErrorRate > 0.01 ? 'var(--severity-warning)' : 'var(--status-good)'} />
        <MetricCard label="Kafka Topics" value={sources.length} subtitle="1:1 mapping to data sources" icon={<Radio size={16} />} accentColor="var(--kafka-accent)" />
      </div>

      <div className="sources-grid">
        {sources.map(source => (
          <div key={source.id} className="source-card card">
            <div className="source-card-header">
              <div className="source-status">
                <span className={`status-dot ${source.status}`} />
                <span className={`source-status-text ${source.status}`}>{source.status}</span>
              </div>
              <Globe size={16} className="source-icon" />
            </div>

            <h3 className="source-name">{source.name}</h3>
            <p className="source-provider">{source.provider}</p>
            <p className="source-description">{source.description}</p>

            <div className="source-details">
              <div className="source-detail">
                <span className="source-detail-label">Data Type</span>
                <span className="source-detail-value">{source.dataType}</span>
              </div>
              <div className="source-detail">
                <span className="source-detail-label">Update Frequency</span>
                <span className="source-detail-value">
                  <RefreshCw size={10} /> {source.updateFrequency}
                </span>
              </div>
              <div className="source-detail">
                <span className="source-detail-label">Kafka Topic</span>
                <code className="source-topic">{source.kafkaTopic}</code>
              </div>
              <div className="source-detail">
                <span className="source-detail-label">Records Ingested</span>
                <span className="source-detail-value">{formatNumber(source.recordsIngested)}</span>
              </div>
              <div className="source-detail">
                <span className="source-detail-label">Error Rate</span>
                <span className="source-detail-value" style={{
                  color: source.errorRate > 0.005 ? 'var(--severity-warning)' : 'var(--status-good)'
                }}>
                  {(source.errorRate * 100).toFixed(2)}%
                </span>
              </div>
              <div className="source-detail">
                <span className="source-detail-label">Last Fetch</span>
                <span className="source-detail-value">
                  <Clock size={10} /> {formatTimeAgo(source.lastFetch)}
                </span>
              </div>
            </div>

            <div className="source-footer">
              <div className="source-endpoint">
                <ExternalLink size={10} />
                <span>{source.apiEndpoint}</span>
              </div>
              <div className="source-health">
                {source.status === 'connected' ? (
                  <><CheckCircle size={12} /> Healthy</>
                ) : (
                  <><AlertTriangle size={12} /> Issue</>
                )}
              </div>
            </div>
          </div>
        ))}
      </div>

      {/* Architecture Diagram */}
      <div className="card architecture-section">
        <h3>Data Pipeline Architecture</h3>
        <div className="architecture-flow">
          <div className="arch-stage">
            <div className="arch-stage-icon" style={{ background: 'var(--accent-bg)', color: 'var(--accent)' }}>
              <Globe size={20} />
            </div>
            <span className="arch-stage-label">Public APIs</span>
            <span className="arch-stage-detail">USGS, NOAA, NASA, EPA, ESA, OpenAQ</span>
          </div>
          <div className="arch-arrow">&rarr;</div>
          <div className="arch-stage">
            <div className="arch-stage-icon" style={{ background: 'rgba(232, 96, 44, 0.1)', color: 'var(--kafka-accent)' }}>
              <Radio size={20} />
            </div>
            <span className="arch-stage-label">Apache Kafka</span>
            <span className="arch-stage-detail">10 topics, 108 partitions, 5 brokers</span>
          </div>
          <div className="arch-arrow">&rarr;</div>
          <div className="arch-stage">
            <div className="arch-stage-icon" style={{ background: 'rgba(245, 158, 11, 0.1)', color: 'var(--spark-accent)' }}>
              <Database size={20} />
            </div>
            <span className="arch-stage-label">Apache Spark</span>
            <span className="arch-stage-detail">Streaming + Batch + ML Inference</span>
          </div>
          <div className="arch-arrow">&rarr;</div>
          <div className="arch-stage">
            <div className="arch-stage-icon" style={{ background: 'rgba(34, 197, 94, 0.1)', color: 'var(--status-good)' }}>
              <CheckCircle size={20} />
            </div>
            <span className="arch-stage-label">Observability</span>
            <span className="arch-stage-detail">Alerts, Maps, Analytics, Risk Scoring</span>
          </div>
        </div>
      </div>
    </div>
  );
}
