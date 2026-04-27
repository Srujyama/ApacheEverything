import { useState, useEffect, useMemo } from 'react';
import {
  Radio, Cpu, Database, ArrowDown, ArrowUp, BarChart3,
  Play, CheckCircle, XCircle, Clock, Zap,
} from 'lucide-react';
import {
  AreaChart, Area, XAxis, YAxis, CartesianGrid, Tooltip, ResponsiveContainer,
  LineChart, Line,
} from 'recharts';
import MetricCard from '../components/common/MetricCard';
import { getKafkaClusterStats, getKafkaTopics, getClusterThroughputHistory, getRecentMessages } from '../services/kafkaService';
import { getSparkClusterStats, getSparkJobs, getSparkThroughputHistory, getSparkMemoryHistory } from '../services/sparkService';
import type { KafkaTopic, SparkJob } from '../types';
import { formatBytes, formatNumber, formatDuration, formatTimestamp } from '../utils/format';
import './DataStreams.css';

export default function DataStreams() {
  const [activeTab, setActiveTab] = useState<'kafka' | 'spark'>('kafka');
  const [selectedTopic, setSelectedTopic] = useState<string | null>(null);
  const [, setTick] = useState(0);

  const kafkaStats = getKafkaClusterStats();
  const kafkaTopics = getKafkaTopics();
  const sparkStats = getSparkClusterStats();
  const sparkJobs = getSparkJobs();
  const kafkaThroughput = getClusterThroughputHistory(40);
  const sparkThroughput = getSparkThroughputHistory(40);
  const sparkMemory = getSparkMemoryHistory(40);

  const messages = useMemo(
    () => selectedTopic ? getRecentMessages(selectedTopic, 15) : [],
    [selectedTopic]
  );

  useEffect(() => {
    const interval = setInterval(() => setTick(t => t + 1), 3000);
    return () => clearInterval(interval);
  }, []);

  function getJobStatusIcon(status: SparkJob['status']) {
    switch (status) {
      case 'running': return <Play size={14} style={{ color: 'var(--status-good)' }} />;
      case 'completed': return <CheckCircle size={14} style={{ color: 'var(--accent)' }} />;
      case 'failed': return <XCircle size={14} style={{ color: 'var(--severity-critical)' }} />;
      case 'pending': return <Clock size={14} style={{ color: 'var(--text-tertiary)' }} />;
      default: return <Clock size={14} />;
    }
  }

  return (
    <div className="data-streams">
      <div className="page-header">
        <div>
          <h1>Data Streams</h1>
          <p className="page-subtitle">Apache Kafka ingestion and Apache Spark processing pipeline</p>
        </div>
        <div className="stream-tabs">
          <button
            className={`stream-tab ${activeTab === 'kafka' ? 'active' : ''}`}
            onClick={() => setActiveTab('kafka')}
          >
            <Radio size={14} /> Kafka
          </button>
          <button
            className={`stream-tab ${activeTab === 'spark' ? 'active' : ''}`}
            onClick={() => setActiveTab('spark')}
          >
            <Cpu size={14} /> Spark
          </button>
        </div>
      </div>

      {activeTab === 'kafka' ? (
        <>
          {/* Kafka Cluster Metrics */}
          <div className="metrics-grid">
            <MetricCard label="Brokers" value={kafkaStats.brokers} icon={<Database size={16} />} accentColor="var(--kafka-accent)" />
            <MetricCard label="Topics" value={kafkaStats.topics} icon={<Radio size={16} />} accentColor="var(--kafka-accent)" />
            <MetricCard label="Partitions" value={kafkaStats.totalPartitions} icon={<BarChart3 size={16} />} accentColor="var(--kafka-accent)" />
            <MetricCard label="Messages/sec" value={formatNumber(kafkaStats.totalMessagesPerSecond)} icon={<ArrowDown size={16} />} trend="up" trendValue="8%" accentColor="var(--kafka-accent)" />
            <MetricCard label="Throughput" value={formatBytes(kafkaStats.totalBytesPerSecond) + '/s'} icon={<ArrowUp size={16} />} accentColor="var(--kafka-accent)" />
            <MetricCard label="Consumer Groups" value={kafkaStats.consumerGroups} icon={<Zap size={16} />} accentColor="var(--kafka-accent)" />
          </div>

          {/* Kafka Throughput Chart */}
          <div className="card">
            <div className="card-header">
              <h3><Radio size={16} /> Cluster Message Throughput</h3>
              <span className="badge badge-success">All brokers healthy</span>
            </div>
            <ResponsiveContainer width="100%" height={220}>
              <AreaChart data={kafkaThroughput.map(p => ({
                time: new Date(p.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
                msgs: Math.round(p.value),
              }))}>
                <defs>
                  <linearGradient id="kGrad" x1="0" y1="0" x2="0" y2="1">
                    <stop offset="5%" stopColor="#e8602c" stopOpacity={0.3} />
                    <stop offset="95%" stopColor="#e8602c" stopOpacity={0} />
                  </linearGradient>
                </defs>
                <CartesianGrid strokeDasharray="3 3" stroke="var(--border-primary)" />
                <XAxis dataKey="time" tick={{ fill: 'var(--text-tertiary)', fontSize: 10 }} tickLine={false} axisLine={false} />
                <YAxis tick={{ fill: 'var(--text-tertiary)', fontSize: 10 }} tickLine={false} axisLine={false} tickFormatter={(v: number) => formatNumber(v)} />
                <Tooltip contentStyle={{ background: 'var(--bg-elevated)', border: '1px solid var(--border-secondary)', borderRadius: 8, fontSize: 12 }} />
                <Area type="monotone" dataKey="msgs" stroke="#e8602c" fill="url(#kGrad)" strokeWidth={2} name="msgs/sec" />
              </AreaChart>
            </ResponsiveContainer>
          </div>

          {/* Kafka Topics Table */}
          <div className="card">
            <div className="card-header">
              <h3><Database size={16} /> Topics</h3>
            </div>
            <div className="table-wrapper">
              <table className="data-table">
                <thead>
                  <tr>
                    <th>Topic</th>
                    <th>Source</th>
                    <th>Partitions</th>
                    <th>Msgs/sec</th>
                    <th>Throughput</th>
                    <th>Lag</th>
                    <th>Status</th>
                    <th>Actions</th>
                  </tr>
                </thead>
                <tbody>
                  {kafkaTopics.map((topic: KafkaTopic) => (
                    <tr key={topic.name} className={selectedTopic === topic.name ? 'row-selected' : ''}>
                      <td>
                        <code>{topic.name}</code>
                      </td>
                      <td className="text-secondary">{topic.source}</td>
                      <td>{topic.partitions}</td>
                      <td>{formatNumber(topic.messagesPerSecond)}</td>
                      <td>{formatBytes(topic.bytesPerSecond)}/s</td>
                      <td>
                        <span style={{ color: topic.lag > 50 ? 'var(--severity-warning)' : 'var(--text-primary)' }}>
                          {topic.lag}
                        </span>
                      </td>
                      <td>
                        <span className={`badge badge-${topic.status === 'active' ? 'success' : 'critical'}`}>
                          {topic.status}
                        </span>
                      </td>
                      <td>
                        <button
                          className="btn-sm"
                          onClick={() => setSelectedTopic(selectedTopic === topic.name ? null : topic.name)}
                        >
                          {selectedTopic === topic.name ? 'Hide' : 'Peek'}
                        </button>
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>

          {/* Message Preview */}
          {selectedTopic && messages.length > 0 && (
            <div className="card">
              <div className="card-header">
                <h3>Latest Messages: <code>{selectedTopic}</code></h3>
              </div>
              <div className="message-stream">
                {messages.map((msg, i) => (
                  <div key={i} className="message-item">
                    <div className="message-meta">
                      <span>P{msg.partition}</span>
                      <span>Offset {msg.offset}</span>
                      <span>{formatTimestamp(msg.timestamp)}</span>
                    </div>
                    <pre className="message-payload">{JSON.stringify(msg.value, null, 2)}</pre>
                  </div>
                ))}
              </div>
            </div>
          )}
        </>
      ) : (
        <>
          {/* Spark Cluster Metrics */}
          <div className="metrics-grid">
            <MetricCard label="Active Jobs" value={sparkStats.activeJobs} icon={<Play size={16} />} accentColor="var(--spark-accent)" />
            <MetricCard label="Completed" value={sparkStats.completedJobs} icon={<CheckCircle size={16} />} accentColor="var(--accent)" />
            <MetricCard label="Failed" value={sparkStats.failedJobs} icon={<XCircle size={16} />} accentColor="var(--severity-critical)" />
            <MetricCard label="Executors" value={`${sparkStats.activeExecutors}/${sparkStats.totalExecutors}`} icon={<Cpu size={16} />} accentColor="var(--spark-accent)" />
            <MetricCard label="Memory Used" value={formatBytes(sparkStats.usedMemory)} subtitle={`of ${formatBytes(sparkStats.totalMemory)}`} icon={<Database size={16} />} accentColor="var(--spark-accent)" />
            <MetricCard label="Avg Batch" value={formatDuration(sparkStats.avgBatchDuration)} icon={<Clock size={16} />} accentColor="var(--spark-accent)" />
          </div>

          {/* Spark Charts */}
          <div className="streams-chart-grid">
            <div className="card">
              <div className="card-header">
                <h3><Cpu size={16} /> Processing Throughput</h3>
              </div>
              <ResponsiveContainer width="100%" height={200}>
                <AreaChart data={sparkThroughput.map(p => ({
                  time: new Date(p.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
                  records: Math.round(p.value),
                }))}>
                  <defs>
                    <linearGradient id="sGrad" x1="0" y1="0" x2="0" y2="1">
                      <stop offset="5%" stopColor="#f59e0b" stopOpacity={0.3} />
                      <stop offset="95%" stopColor="#f59e0b" stopOpacity={0} />
                    </linearGradient>
                  </defs>
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--border-primary)" />
                  <XAxis dataKey="time" tick={{ fill: 'var(--text-tertiary)', fontSize: 10 }} tickLine={false} axisLine={false} />
                  <YAxis tick={{ fill: 'var(--text-tertiary)', fontSize: 10 }} tickLine={false} axisLine={false} tickFormatter={(v: number) => formatNumber(v)} />
                  <Tooltip contentStyle={{ background: 'var(--bg-elevated)', border: '1px solid var(--border-secondary)', borderRadius: 8, fontSize: 12 }} />
                  <Area type="monotone" dataKey="records" stroke="#f59e0b" fill="url(#sGrad)" strokeWidth={2} name="records/sec" />
                </AreaChart>
              </ResponsiveContainer>
            </div>

            <div className="card">
              <div className="card-header">
                <h3><Database size={16} /> Memory Usage (GB)</h3>
              </div>
              <ResponsiveContainer width="100%" height={200}>
                <LineChart data={sparkMemory.map(p => ({
                  time: new Date(p.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
                  gb: p.value,
                }))}>
                  <CartesianGrid strokeDasharray="3 3" stroke="var(--border-primary)" />
                  <XAxis dataKey="time" tick={{ fill: 'var(--text-tertiary)', fontSize: 10 }} tickLine={false} axisLine={false} />
                  <YAxis tick={{ fill: 'var(--text-tertiary)', fontSize: 10 }} tickLine={false} axisLine={false} />
                  <Tooltip contentStyle={{ background: 'var(--bg-elevated)', border: '1px solid var(--border-secondary)', borderRadius: 8, fontSize: 12 }} />
                  <Line type="monotone" dataKey="gb" stroke="var(--chart-2)" strokeWidth={2} dot={false} name="GB Used" />
                </LineChart>
              </ResponsiveContainer>
            </div>
          </div>

          {/* Spark Jobs Table */}
          <div className="card">
            <div className="card-header">
              <h3><Zap size={16} /> Spark Jobs</h3>
            </div>
            <div className="spark-jobs">
              {sparkJobs.map((job: SparkJob) => (
                <div key={job.id} className={`spark-job-card severity-border-${job.status}`}>
                  <div className="spark-job-header">
                    <div className="spark-job-title">
                      {getJobStatusIcon(job.status)}
                      <span>{job.name}</span>
                      <span className={`badge badge-${job.status === 'running' ? 'success' : job.status === 'failed' ? 'critical' : job.status === 'completed' ? 'info' : 'warning'}`}>
                        {job.status}
                      </span>
                    </div>
                    <code className="spark-job-id">{job.id}</code>
                  </div>
                  <p className="spark-job-desc">{job.description}</p>
                  <div className="spark-job-meta">
                    <span>Type: <strong>{job.type.replace(/_/g, ' ')}</strong></span>
                    <span>Stages: <strong>{job.completedStages}/{job.stages}</strong></span>
                    <span>Executors: <strong>{job.executors}</strong></span>
                    <span>Input: <strong>{formatNumber(job.inputRecords)} records</strong></span>
                    <span>Output: <strong>{formatNumber(job.outputRecords)} records</strong></span>
                    {job.memoryUsed > 0 && <span>Memory: <strong>{formatBytes(job.memoryUsed)}</strong></span>}
                  </div>
                  <div className="spark-job-topics">
                    {job.inputTopics.map(t => <code key={t}>{t}</code>)}
                  </div>
                  {job.status === 'running' && (
                    <div className="progress-bar" style={{ marginTop: 8 }}>
                      <div className="progress-bar-fill" style={{ width: `${job.progress}%`, background: 'var(--spark-accent)' }} />
                    </div>
                  )}
                  {job.status === 'running' && (
                    <span className="spark-job-progress">{job.progress}%</span>
                  )}
                </div>
              ))}
            </div>
          </div>
        </>
      )}
    </div>
  );
}
