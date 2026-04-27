import { useState, useEffect } from 'react';
import {
  Activity,
  Radio,
  AlertTriangle,
  Cpu,
  Database,
  Satellite,
  Shield,
  Clock,
  ArrowRight,
  Flame,
} from 'lucide-react';
import {
  AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
  BarChart, Bar,
} from 'recharts';
import MetricCard from '../components/common/MetricCard';
import { getKafkaClusterStats, getClusterThroughputHistory } from '../services/kafkaService';
import { getSparkClusterStats } from '../services/sparkService';
import { getCriticalAndEmergencyAlerts, getAlertCounts } from '../services/alertService';
import { getSensorsByStatus } from '../services/sensorService';
import { fetchActiveWeatherAlerts } from '../services/publicDataSources';
import type { Alert } from '../types';
import { formatTimeAgo, formatNumber, formatBytes } from '../utils/format';
import './Dashboard.css';

export default function Dashboard() {
  const [weatherAlerts, setWeatherAlerts] = useState<Alert[]>([]);
  const [tickCount, setTickCount] = useState(0);

  const kafkaStats = getKafkaClusterStats();
  const sparkStats = getSparkClusterStats();
  const alertCounts = getAlertCounts();
  const sensorStatus = getSensorsByStatus();
  const criticalAlerts = getCriticalAndEmergencyAlerts();
  const throughputHistory = getClusterThroughputHistory(30);

  useEffect(() => {
    fetchActiveWeatherAlerts().then(setWeatherAlerts);
  }, []);

  // Simulate live tick for real-time feel
  useEffect(() => {
    const interval = setInterval(() => setTickCount(c => c + 1), 5000);
    return () => clearInterval(interval);
  }, []);

  // Force re-render context
  void tickCount;

  const allAlerts = [...criticalAlerts, ...weatherAlerts.filter(a => a.severity === 'critical' || a.severity === 'emergency')].slice(0, 8);

  const alertDistribution = [
    { name: 'Emergency', count: alertCounts['emergency'] || 0, fill: 'var(--severity-emergency)' },
    { name: 'Critical', count: alertCounts['critical'] || 0, fill: 'var(--severity-critical)' },
    { name: 'Warning', count: alertCounts['warning'] || 0, fill: 'var(--severity-warning)' },
    { name: 'Info', count: alertCounts['info'] || 0, fill: 'var(--severity-info)' },
  ];

  return (
    <div className="dashboard">
      <div className="page-header">
        <div>
          <h1>Physical Observability</h1>
          <p className="page-subtitle">
            Real-time monitoring of physical infrastructure across the United States
          </p>
        </div>
        <div className="live-indicator">
          <span className="status-dot online pulse-dot" />
          <span>LIVE</span>
          <Clock size={14} />
          <span>{new Date().toLocaleTimeString()}</span>
        </div>
      </div>

      {/* Top Metrics Row */}
      <div className="metrics-grid">
        <MetricCard
          label="Active Sensors"
          value={sensorStatus.online}
          subtitle={`${sensorStatus.offline} offline, ${sensorStatus.degraded} degraded`}
          icon={<Activity size={16} />}
          accentColor="var(--status-good)"
        />
        <MetricCard
          label="Kafka Messages/sec"
          value={formatNumber(kafkaStats.totalMessagesPerSecond)}
          subtitle={`${kafkaStats.topics} topics, ${kafkaStats.brokers} brokers`}
          icon={<Radio size={16} />}
          trend="up"
          trendValue="12%"
          accentColor="var(--kafka-accent)"
        />
        <MetricCard
          label="Spark Jobs Running"
          value={sparkStats.activeJobs}
          subtitle={`${sparkStats.activeExecutors}/${sparkStats.totalExecutors} executors`}
          icon={<Cpu size={16} />}
          accentColor="var(--spark-accent)"
        />
        <MetricCard
          label="Critical Alerts"
          value={(alertCounts['emergency'] || 0) + (alertCounts['critical'] || 0)}
          subtitle={`${(alertCounts['warning'] || 0)} warnings active`}
          icon={<AlertTriangle size={16} />}
          accentColor="var(--severity-emergency)"
        />
        <MetricCard
          label="Data Ingested Today"
          value="2.4 TB"
          subtitle={`${formatBytes(kafkaStats.totalBytesPerSecond)}/s throughput`}
          icon={<Database size={16} />}
          trend="up"
          trendValue="8%"
          accentColor="var(--accent)"
        />
        <MetricCard
          label="Data Sources"
          value={8}
          subtitle="USGS, NOAA, NASA, EPA, ESA..."
          icon={<Satellite size={16} />}
          accentColor="var(--chart-3)"
        />
      </div>

      {/* Main Content Grid */}
      <div className="dashboard-grid">
        {/* Kafka Throughput Chart */}
        <div className="card dashboard-chart">
          <div className="card-header">
            <h3>
              <Radio size={16} />
              Kafka Cluster Throughput
            </h3>
            <span className="badge badge-success">
              <span className="status-dot online" /> Healthy
            </span>
          </div>
          <ResponsiveContainer width="100%" height={200}>
            <AreaChart data={throughputHistory.map(p => ({
              time: new Date(p.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
              msgs: Math.round(p.value),
            }))}>
              <defs>
                <linearGradient id="kafkaGradient" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor="var(--kafka-accent)" stopOpacity={0.3} />
                  <stop offset="95%" stopColor="var(--kafka-accent)" stopOpacity={0} />
                </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--border-primary)" />
              <XAxis dataKey="time" tick={{ fill: 'var(--text-tertiary)', fontSize: 10 }} tickLine={false} axisLine={false} />
              <YAxis tick={{ fill: 'var(--text-tertiary)', fontSize: 10 }} tickLine={false} axisLine={false} tickFormatter={(v: number) => formatNumber(v)} />
              <Tooltip
                contentStyle={{ background: 'var(--bg-elevated)', border: '1px solid var(--border-secondary)', borderRadius: 8, fontSize: 12 }}
                labelStyle={{ color: 'var(--text-secondary)' }}
              />
              <Area type="monotone" dataKey="msgs" stroke="var(--kafka-accent)" fill="url(#kafkaGradient)" strokeWidth={2} name="msgs/sec" />
            </AreaChart>
          </ResponsiveContainer>
        </div>

        {/* Alert Distribution */}
        <div className="card dashboard-alert-dist">
          <div className="card-header">
            <h3>
              <Shield size={16} />
              Alert Distribution
            </h3>
          </div>
          <ResponsiveContainer width="100%" height={200}>
            <BarChart data={alertDistribution} layout="vertical">
              <CartesianGrid strokeDasharray="3 3" stroke="var(--border-primary)" />
              <XAxis type="number" tick={{ fill: 'var(--text-tertiary)', fontSize: 10 }} tickLine={false} axisLine={false} />
              <YAxis dataKey="name" type="category" tick={{ fill: 'var(--text-tertiary)', fontSize: 11 }} tickLine={false} axisLine={false} width={80} />
              <Tooltip
                contentStyle={{ background: 'var(--bg-elevated)', border: '1px solid var(--border-secondary)', borderRadius: 8, fontSize: 12 }}
              />
              <Bar dataKey="count" radius={[0, 4, 4, 0]} barSize={20}>
                {alertDistribution.map((entry, index) => (
                  <rect key={index} fill={entry.fill} />
                ))}
              </Bar>
            </BarChart>
          </ResponsiveContainer>
        </div>

        {/* Critical Alert Feed */}
        <div className="card dashboard-alerts">
          <div className="card-header">
            <h3>
              <Flame size={16} />
              Active Critical Alerts
            </h3>
            <span className="text-link">
              View All <ArrowRight size={14} />
            </span>
          </div>
          <div className="alert-feed">
            {allAlerts.length === 0 && (
              <p className="empty-state">No critical alerts - all systems nominal</p>
            )}
            {allAlerts.map((alert) => (
              <div key={alert.id} className={`alert-feed-item severity-${alert.severity}`}>
                <div className="alert-feed-indicator">
                  <span className={`status-dot ${alert.severity === 'emergency' ? 'offline' : 'degraded'} pulse-dot`} />
                </div>
                <div className="alert-feed-content">
                  <div className="alert-feed-title">
                    <span className={`badge badge-${alert.severity}`}>{alert.severity}</span>
                    <span className="alert-feed-source">{alert.source}</span>
                  </div>
                  <p className="alert-feed-text">{alert.title}</p>
                  <p className="alert-feed-desc">{alert.description.slice(0, 120)}...</p>
                  <div className="alert-feed-meta">
                    <span>{formatTimeAgo(alert.timestamp)}</span>
                    {alert.tags.slice(0, 3).map(tag => (
                      <code key={tag}>{tag}</code>
                    ))}
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>

        {/* Pipeline Status */}
        <div className="card dashboard-pipeline">
          <div className="card-header">
            <h3>
              <Cpu size={16} />
              Processing Pipeline
            </h3>
          </div>
          <div className="pipeline-flow">
            <div className="pipeline-stage">
              <div className="pipeline-stage-header">
                <Radio size={14} />
                <span>Ingest (Kafka)</span>
              </div>
              <div className="pipeline-stage-stats">
                <div className="pipeline-stat">
                  <span className="pipeline-stat-value">{formatNumber(kafkaStats.totalMessagesPerSecond)}</span>
                  <span className="pipeline-stat-label">msgs/sec</span>
                </div>
                <div className="pipeline-stat">
                  <span className="pipeline-stat-value">{kafkaStats.topics}</span>
                  <span className="pipeline-stat-label">topics</span>
                </div>
                <div className="pipeline-stat">
                  <span className="pipeline-stat-value">{kafkaStats.totalPartitions}</span>
                  <span className="pipeline-stat-label">partitions</span>
                </div>
              </div>
              <div className="progress-bar">
                <div className="progress-bar-fill" style={{ width: '85%', background: 'var(--kafka-accent)' }} />
              </div>
            </div>

            <div className="pipeline-arrow">
              <ArrowRight size={20} />
            </div>

            <div className="pipeline-stage">
              <div className="pipeline-stage-header">
                <Cpu size={14} />
                <span>Process (Spark)</span>
              </div>
              <div className="pipeline-stage-stats">
                <div className="pipeline-stat">
                  <span className="pipeline-stat-value">{sparkStats.activeJobs}</span>
                  <span className="pipeline-stat-label">jobs</span>
                </div>
                <div className="pipeline-stat">
                  <span className="pipeline-stat-value">{sparkStats.activeCores}</span>
                  <span className="pipeline-stat-label">cores</span>
                </div>
                <div className="pipeline-stat">
                  <span className="pipeline-stat-value">{formatBytes(sparkStats.usedMemory)}</span>
                  <span className="pipeline-stat-label">memory</span>
                </div>
              </div>
              <div className="progress-bar">
                <div className="progress-bar-fill" style={{ width: `${(sparkStats.usedMemory / sparkStats.totalMemory) * 100}%`, background: 'var(--spark-accent)' }} />
              </div>
            </div>

            <div className="pipeline-arrow">
              <ArrowRight size={20} />
            </div>

            <div className="pipeline-stage">
              <div className="pipeline-stage-header">
                <AlertTriangle size={14} />
                <span>Detect & Alert</span>
              </div>
              <div className="pipeline-stage-stats">
                <div className="pipeline-stat">
                  <span className="pipeline-stat-value">6</span>
                  <span className="pipeline-stat-label">AI models</span>
                </div>
                <div className="pipeline-stat">
                  <span className="pipeline-stat-value">{Object.values(alertCounts).reduce((a, b) => a + b, 0)}</span>
                  <span className="pipeline-stat-label">alerts</span>
                </div>
                <div className="pipeline-stat">
                  <span className="pipeline-stat-value">99.7%</span>
                  <span className="pipeline-stat-label">uptime</span>
                </div>
              </div>
              <div className="progress-bar">
                <div className="progress-bar-fill" style={{ width: '99.7%', background: 'var(--status-good)' }} />
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
