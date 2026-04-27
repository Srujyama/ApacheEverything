// ============================================================================
// Physical Observability Platform - Core Type Definitions
// ============================================================================

// --- Geospatial ---
export interface GeoCoordinate {
  lat: number;
  lng: number;
  altitude?: number;
}

export interface GeoBounds {
  north: number;
  south: number;
  east: number;
  west: number;
}

// --- Sensor & Infrastructure ---
export type AssetCategory =
  | 'bridge'
  | 'power_line'
  | 'pipeline'
  | 'dam'
  | 'waterway'
  | 'forest'
  | 'cropland'
  | 'building'
  | 'road'
  | 'weather_station';

export type SensorType =
  | 'seismic'
  | 'temperature'
  | 'humidity'
  | 'air_quality'
  | 'water_level'
  | 'wind_speed'
  | 'camera'
  | 'gas_detector'
  | 'structural_strain'
  | 'soil_moisture'
  | 'radiation'
  | 'acoustic';

export type AssetCondition = 'optimal' | 'good' | 'fair' | 'degraded' | 'critical';

export interface Sensor {
  id: string;
  name: string;
  type: SensorType;
  location: GeoCoordinate;
  status: 'online' | 'offline' | 'degraded' | 'maintenance';
  lastReading: number;
  unit: string;
  lastUpdated: string;
  assetId?: string;
}

export interface InfrastructureAsset {
  id: string;
  name: string;
  category: AssetCategory;
  location: GeoCoordinate;
  condition: AssetCondition;
  sensors: string[]; // sensor IDs
  lastInspection: string;
  nextInspection: string;
  riskScore: number; // 0-100
  metadata: Record<string, string | number>;
}

// --- Alerts ---
export type AlertSeverity = 'info' | 'warning' | 'critical' | 'emergency';
export type AlertStatus = 'active' | 'acknowledged' | 'resolved' | 'expired';
export type AlertSource = 'sensor' | 'ai_model' | 'public_api' | 'manual' | 'spark_analytics';

export interface Alert {
  id: string;
  title: string;
  description: string;
  severity: AlertSeverity;
  status: AlertStatus;
  source: AlertSource;
  location?: GeoCoordinate;
  assetId?: string;
  sensorId?: string;
  timestamp: string;
  acknowledgedAt?: string;
  resolvedAt?: string;
  tags: string[];
}

// --- Kafka Data Streams ---
export type KafkaTopicCategory =
  | 'seismic_events'
  | 'weather_data'
  | 'air_quality'
  | 'water_levels'
  | 'fire_detections'
  | 'structural_health'
  | 'satellite_imagery'
  | 'gas_emissions'
  | 'soil_conditions'
  | 'traffic_flow';

export interface KafkaTopic {
  name: string;
  category: KafkaTopicCategory;
  partitions: number;
  replicationFactor: number;
  messagesPerSecond: number;
  bytesPerSecond: number;
  consumerGroups: number;
  lag: number;
  status: 'active' | 'inactive' | 'error';
  retentionHours: number;
  source: string; // e.g., "USGS API", "EPA AirNow", etc.
}

export interface KafkaClusterStats {
  brokers: number;
  topics: number;
  totalPartitions: number;
  totalMessagesPerSecond: number;
  totalBytesPerSecond: number;
  consumerGroups: number;
  underReplicatedPartitions: number;
  offlinePartitions: number;
}

export interface KafkaMessage {
  topic: string;
  partition: number;
  offset: number;
  timestamp: string;
  key: string;
  value: Record<string, unknown>;
}

// --- Spark Processing ---
export type SparkJobStatus = 'running' | 'completed' | 'failed' | 'pending' | 'cancelled';
export type SparkJobType =
  | 'batch_analysis'
  | 'stream_processing'
  | 'ml_inference'
  | 'anomaly_detection'
  | 'trend_analysis'
  | 'geospatial_correlation'
  | 'risk_scoring';

export interface SparkJob {
  id: string;
  name: string;
  type: SparkJobType;
  status: SparkJobStatus;
  progress: number; // 0-100
  startedAt: string;
  completedAt?: string;
  duration?: number; // ms
  inputRecords: number;
  outputRecords: number;
  stages: number;
  completedStages: number;
  executors: number;
  memoryUsed: number; // bytes
  shuffleRead: number; // bytes
  shuffleWrite: number; // bytes
  inputTopics: string[];
  description: string;
}

export interface SparkClusterStats {
  activeJobs: number;
  completedJobs: number;
  failedJobs: number;
  totalExecutors: number;
  activeExecutors: number;
  totalMemory: number;
  usedMemory: number;
  totalCores: number;
  activeCores: number;
  streamingBatches: number;
  avgBatchDuration: number;
}

// --- Public Infrastructure Data Sources ---
export interface PublicDataSource {
  id: string;
  name: string;
  provider: string;
  description: string;
  apiEndpoint: string;
  kafkaTopic: string;
  dataType: string;
  updateFrequency: string;
  status: 'connected' | 'disconnected' | 'error' | 'rate_limited';
  lastFetch: string;
  recordsIngested: number;
  errorRate: number;
}

// --- Time Series ---
export interface TimeSeriesPoint {
  timestamp: string;
  value: number;
}

export interface TimeSeriesData {
  metric: string;
  unit: string;
  points: TimeSeriesPoint[];
}

// --- Dashboard Metrics ---
export interface PlatformMetrics {
  totalSensors: number;
  activeSensors: number;
  totalAssets: number;
  criticalAlerts: number;
  warningAlerts: number;
  kafkaMessagesPerSec: number;
  sparkJobsRunning: number;
  dataSourcesConnected: number;
  totalDataIngested: string; // human-readable, e.g., "2.4 TB"
  avgResponseTime: number; // ms
  aiModelsActive: number;
  uptime: number; // percentage
}

// --- Map ---
export interface MapLayer {
  id: string;
  name: string;
  type: 'sensors' | 'assets' | 'alerts' | 'heatmap' | 'satellite';
  visible: boolean;
  opacity: number;
}

export interface MapMarker {
  id: string;
  position: GeoCoordinate;
  type: 'sensor' | 'asset' | 'alert' | 'fire' | 'earthquake' | 'flood';
  severity?: AlertSeverity;
  label: string;
  details: Record<string, string | number>;
}
