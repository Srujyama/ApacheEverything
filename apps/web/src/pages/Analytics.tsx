import { Cpu, TrendingUp, Droplets, Flame, Wind, Building2 } from 'lucide-react';
import {
  AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
  LineChart, Line, BarChart, Bar,
} from 'recharts';
import {
  getAirQualityTrends,
  getSeismicActivityTrends,
  getWaterLevelTrends,
  getFireDetectionTrends,
  getInfrastructureRiskDistribution,
} from '../services/sparkService';
import { formatNumber } from '../utils/format';
import './Analytics.css';

export default function Analytics() {
  const airQuality = getAirQualityTrends();
  const seismic = getSeismicActivityTrends();
  const waterLevel = getWaterLevelTrends();
  const fires = getFireDetectionTrends();
  const riskDist = getInfrastructureRiskDistribution();

  return (
    <div className="analytics-page">
      <div className="page-header">
        <div>
          <h1>Analytics</h1>
          <p className="page-subtitle">
            Trends and insights powered by Apache Spark batch and streaming analysis
          </p>
        </div>
        <div className="spark-powered-badge">
          <Cpu size={14} />
          Processed by Apache Spark
        </div>
      </div>

      <div className="analytics-grid">
        {/* Air Quality Trends */}
        <div className="card analytics-chart">
          <div className="card-header">
            <h3><Wind size={16} /> {airQuality.metric}</h3>
            <span className="chart-period">90-day trend</span>
          </div>
          <ResponsiveContainer width="100%" height={240}>
            <AreaChart data={airQuality.points.map(p => ({
              date: new Date(p.timestamp).toLocaleDateString('en-US', { month: 'short', day: 'numeric' }),
              value: Math.round(p.value),
            }))}>
              <defs>
                <linearGradient id="aqiGrad" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor="var(--chart-1)" stopOpacity={0.3} />
                  <stop offset="95%" stopColor="var(--chart-1)" stopOpacity={0} />
                </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--border-primary)" />
              <XAxis dataKey="date" tick={{ fill: 'var(--text-tertiary)', fontSize: 10 }} tickLine={false} axisLine={false} interval={14} />
              <YAxis tick={{ fill: 'var(--text-tertiary)', fontSize: 10 }} tickLine={false} axisLine={false} />
              <Tooltip contentStyle={{ background: 'var(--bg-elevated)', border: '1px solid var(--border-secondary)', borderRadius: 8, fontSize: 12 }} />
              <Area type="monotone" dataKey="value" stroke="var(--chart-1)" fill="url(#aqiGrad)" strokeWidth={2} name="AQI" />
            </AreaChart>
          </ResponsiveContainer>
          <div className="chart-insight">
            <TrendingUp size={14} />
            <span>AQI trending upward in the last 30 days - smoke season impact detected by Spark trend analysis</span>
          </div>
        </div>

        {/* Seismic Activity */}
        <div className="card analytics-chart">
          <div className="card-header">
            <h3><TrendingUp size={16} /> {seismic.metric}</h3>
            <span className="chart-period">30-day window</span>
          </div>
          <ResponsiveContainer width="100%" height={240}>
            <BarChart data={seismic.points.map(p => ({
              date: new Date(p.timestamp).toLocaleDateString('en-US', { month: 'short', day: 'numeric' }),
              events: Math.round(p.value),
            }))}>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--border-primary)" />
              <XAxis dataKey="date" tick={{ fill: 'var(--text-tertiary)', fontSize: 10 }} tickLine={false} axisLine={false} interval={4} />
              <YAxis tick={{ fill: 'var(--text-tertiary)', fontSize: 10 }} tickLine={false} axisLine={false} />
              <Tooltip contentStyle={{ background: 'var(--bg-elevated)', border: '1px solid var(--border-secondary)', borderRadius: 8, fontSize: 12 }} />
              <Bar dataKey="events" fill="var(--chart-2)" radius={[2, 2, 0, 0]} name="Earthquakes" />
            </BarChart>
          </ResponsiveContainer>
          <div className="chart-insight">
            <Cpu size={14} />
            <span>Spark anomaly detection: seismic cluster identified in California subduction zone - 23% above baseline</span>
          </div>
        </div>

        {/* Water Level */}
        <div className="card analytics-chart">
          <div className="card-header">
            <h3><Droplets size={16} /> {waterLevel.metric}</h3>
            <span className="chart-period">7-day hourly</span>
          </div>
          <ResponsiveContainer width="100%" height={240}>
            <LineChart data={waterLevel.points.filter((_, i) => i % 4 === 0).map(p => ({
              time: new Date(p.timestamp).toLocaleDateString('en-US', { weekday: 'short', hour: '2-digit' }),
              level: +p.value.toFixed(1),
            }))}>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--border-primary)" />
              <XAxis dataKey="time" tick={{ fill: 'var(--text-tertiary)', fontSize: 10 }} tickLine={false} axisLine={false} interval={8} />
              <YAxis tick={{ fill: 'var(--text-tertiary)', fontSize: 10 }} tickLine={false} axisLine={false} domain={['auto', 'auto']} />
              <Tooltip contentStyle={{ background: 'var(--bg-elevated)', border: '1px solid var(--border-secondary)', borderRadius: 8, fontSize: 12 }} />
              <Line type="monotone" dataKey="level" stroke="var(--chart-3)" strokeWidth={2} dot={false} name="Gauge Height (ft)" />
            </LineChart>
          </ResponsiveContainer>
          <div className="chart-insight">
            <Droplets size={14} />
            <span>USGS stream data processed via Kafka + Spark. Tidal and seasonal patterns detected.</span>
          </div>
        </div>

        {/* Fire Detections */}
        <div className="card analytics-chart">
          <div className="card-header">
            <h3><Flame size={16} /> {fires.metric}</h3>
            <span className="chart-period">30-day trend</span>
          </div>
          <ResponsiveContainer width="100%" height={240}>
            <AreaChart data={fires.points.map(p => ({
              date: new Date(p.timestamp).toLocaleDateString('en-US', { month: 'short', day: 'numeric' }),
              fires: Math.round(p.value),
            }))}>
              <defs>
                <linearGradient id="fireGrad" x1="0" y1="0" x2="0" y2="1">
                  <stop offset="5%" stopColor="var(--severity-critical)" stopOpacity={0.3} />
                  <stop offset="95%" stopColor="var(--severity-critical)" stopOpacity={0} />
                </linearGradient>
              </defs>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--border-primary)" />
              <XAxis dataKey="date" tick={{ fill: 'var(--text-tertiary)', fontSize: 10 }} tickLine={false} axisLine={false} interval={4} />
              <YAxis tick={{ fill: 'var(--text-tertiary)', fontSize: 10 }} tickLine={false} axisLine={false} />
              <Tooltip contentStyle={{ background: 'var(--bg-elevated)', border: '1px solid var(--border-secondary)', borderRadius: 8, fontSize: 12 }} />
              <Area type="monotone" dataKey="fires" stroke="var(--severity-critical)" fill="url(#fireGrad)" strokeWidth={2} name="Active Fires" />
            </AreaChart>
          </ResponsiveContainer>
          <div className="chart-insight">
            <Flame size={14} />
            <span>NASA FIRMS data shows 40% increase in last 10 days. Spark ML model predicts continued escalation.</span>
          </div>
        </div>

        {/* Infrastructure Risk Distribution */}
        <div className="card analytics-chart-full">
          <div className="card-header">
            <h3><Building2 size={16} /> National Infrastructure Risk Assessment</h3>
            <span className="chart-period">Spark batch analysis</span>
          </div>
          <ResponsiveContainer width="100%" height={280}>
            <BarChart data={riskDist}>
              <CartesianGrid strokeDasharray="3 3" stroke="var(--border-primary)" />
              <XAxis dataKey="category" tick={{ fill: 'var(--text-tertiary)', fontSize: 11 }} tickLine={false} axisLine={false} />
              <YAxis yAxisId="count" tick={{ fill: 'var(--text-tertiary)', fontSize: 10 }} tickLine={false} axisLine={false} tickFormatter={(v: number) => formatNumber(v)} />
              <YAxis yAxisId="risk" orientation="right" tick={{ fill: 'var(--text-tertiary)', fontSize: 10 }} tickLine={false} axisLine={false} domain={[0, 100]} />
              <Tooltip contentStyle={{ background: 'var(--bg-elevated)', border: '1px solid var(--border-secondary)', borderRadius: 8, fontSize: 12 }} />
              <Bar yAxisId="count" dataKey="count" fill="var(--chart-6)" radius={[4, 4, 0, 0]} name="Total Assets" />
              <Line yAxisId="risk" type="monotone" dataKey="avgRisk" stroke="var(--severity-warning)" strokeWidth={3} dot={{ fill: 'var(--severity-warning)', r: 5 }} name="Avg Risk Score" />
            </BarChart>
          </ResponsiveContainer>
          <div className="chart-insight">
            <Building2 size={14} />
            <span>Pipelines show highest average risk (41/100). 2.7M miles of pipeline monitored via Kafka ingestion from DOT/EPA sources.</span>
          </div>
        </div>
      </div>
    </div>
  );
}
