// ============================================================================
// Apache Kafka Data Ingestion Service
// Simulates Kafka cluster metrics and topic management for the frontend
// In production, this would connect to Kafka REST Proxy or a backend WebSocket
// ============================================================================

import type {
  KafkaTopic,
  KafkaClusterStats,
  KafkaMessage,
  TimeSeriesPoint,
} from '../types';

// --- Kafka Topics (mapped to real public data sources) ---
const KAFKA_TOPICS: KafkaTopic[] = [
  {
    name: 'physical-obs.seismic-events',
    category: 'seismic_events',
    partitions: 12,
    replicationFactor: 3,
    messagesPerSecond: 47,
    bytesPerSecond: 23_400,
    consumerGroups: 3,
    lag: 12,
    status: 'active',
    retentionHours: 168,
    source: 'USGS Earthquake Hazards Program',
  },
  {
    name: 'physical-obs.weather-data',
    category: 'weather_data',
    partitions: 24,
    replicationFactor: 3,
    messagesPerSecond: 312,
    bytesPerSecond: 156_000,
    consumerGroups: 5,
    lag: 3,
    status: 'active',
    retentionHours: 72,
    source: 'NOAA National Weather Service',
  },
  {
    name: 'physical-obs.air-quality',
    category: 'air_quality',
    partitions: 8,
    replicationFactor: 3,
    messagesPerSecond: 89,
    bytesPerSecond: 44_500,
    consumerGroups: 4,
    lag: 0,
    status: 'active',
    retentionHours: 168,
    source: 'EPA AirNow / OpenAQ',
  },
  {
    name: 'physical-obs.water-levels',
    category: 'water_levels',
    partitions: 16,
    replicationFactor: 3,
    messagesPerSecond: 156,
    bytesPerSecond: 78_000,
    consumerGroups: 3,
    lag: 7,
    status: 'active',
    retentionHours: 336,
    source: 'USGS Water Services',
  },
  {
    name: 'physical-obs.fire-detections',
    category: 'fire_detections',
    partitions: 6,
    replicationFactor: 3,
    messagesPerSecond: 23,
    bytesPerSecond: 34_500,
    consumerGroups: 4,
    lag: 1,
    status: 'active',
    retentionHours: 720,
    source: 'NASA FIRMS (MODIS/VIIRS)',
  },
  {
    name: 'physical-obs.structural-health',
    category: 'structural_health',
    partitions: 8,
    replicationFactor: 3,
    messagesPerSecond: 234,
    bytesPerSecond: 117_000,
    consumerGroups: 2,
    lag: 45,
    status: 'active',
    retentionHours: 720,
    source: 'Bridge & Infrastructure Sensors',
  },
  {
    name: 'physical-obs.satellite-imagery',
    category: 'satellite_imagery',
    partitions: 4,
    replicationFactor: 3,
    messagesPerSecond: 2,
    bytesPerSecond: 5_200_000,
    consumerGroups: 2,
    lag: 0,
    status: 'active',
    retentionHours: 2160,
    source: 'Copernicus Sentinel / Landsat',
  },
  {
    name: 'physical-obs.gas-emissions',
    category: 'gas_emissions',
    partitions: 6,
    replicationFactor: 3,
    messagesPerSecond: 67,
    bytesPerSecond: 33_500,
    consumerGroups: 3,
    lag: 2,
    status: 'active',
    retentionHours: 168,
    source: 'EPA Greenhouse Gas Reporting',
  },
  {
    name: 'physical-obs.soil-conditions',
    category: 'soil_conditions',
    partitions: 8,
    replicationFactor: 3,
    messagesPerSecond: 45,
    bytesPerSecond: 22_500,
    consumerGroups: 2,
    lag: 18,
    status: 'active',
    retentionHours: 720,
    source: 'USDA NRCS Soil Data',
  },
  {
    name: 'physical-obs.traffic-flow',
    category: 'traffic_flow',
    partitions: 16,
    replicationFactor: 3,
    messagesPerSecond: 1_240,
    bytesPerSecond: 620_000,
    consumerGroups: 3,
    lag: 89,
    status: 'active',
    retentionHours: 48,
    source: 'DOT Traffic Sensors',
  },
];

// --- Kafka Cluster Stats ---
export function getKafkaClusterStats(): KafkaClusterStats {
  const topics = getKafkaTopics();
  return {
    brokers: 5,
    topics: topics.length,
    totalPartitions: topics.reduce((sum, t) => sum + t.partitions, 0),
    totalMessagesPerSecond: topics.reduce((sum, t) => sum + t.messagesPerSecond, 0),
    totalBytesPerSecond: topics.reduce((sum, t) => sum + t.bytesPerSecond, 0),
    consumerGroups: 12,
    underReplicatedPartitions: 2,
    offlinePartitions: 0,
  };
}

// --- Get topics with real-time-ish jitter ---
export function getKafkaTopics(): KafkaTopic[] {
  return KAFKA_TOPICS.map(topic => ({
    ...topic,
    messagesPerSecond: topic.messagesPerSecond + Math.floor(Math.random() * topic.messagesPerSecond * 0.2 - topic.messagesPerSecond * 0.1),
    bytesPerSecond: topic.bytesPerSecond + Math.floor(Math.random() * topic.bytesPerSecond * 0.2 - topic.bytesPerSecond * 0.1),
    lag: Math.max(0, topic.lag + Math.floor(Math.random() * 10 - 5)),
  }));
}

// --- Simulate recent messages from a topic ---
export function getRecentMessages(topicName: string, count: number = 20): KafkaMessage[] {
  const now = Date.now();
  const topic = KAFKA_TOPICS.find(t => t.name === topicName);
  if (!topic) return [];

  return Array.from({ length: count }, (_, i) => ({
    topic: topicName,
    partition: Math.floor(Math.random() * topic.partitions),
    offset: 1_000_000 + i,
    timestamp: new Date(now - (count - i) * 1000).toISOString(),
    key: generateMessageKey(topic.category),
    value: generateMessageValue(topic.category),
  }));
}

// --- Throughput time series (for charts) ---
export function getTopicThroughputHistory(topicName: string, points: number = 60): TimeSeriesPoint[] {
  const topic = KAFKA_TOPICS.find(t => t.name === topicName);
  const baseMps = topic?.messagesPerSecond ?? 100;
  const now = Date.now();

  return Array.from({ length: points }, (_, i) => ({
    timestamp: new Date(now - (points - i) * 60000).toISOString(),
    value: Math.max(0, baseMps + Math.sin(i / 8) * baseMps * 0.3 + (Math.random() - 0.5) * baseMps * 0.2),
  }));
}

export function getClusterThroughputHistory(points: number = 60): TimeSeriesPoint[] {
  const totalMps = KAFKA_TOPICS.reduce((sum, t) => sum + t.messagesPerSecond, 0);
  const now = Date.now();

  return Array.from({ length: points }, (_, i) => ({
    timestamp: new Date(now - (points - i) * 60000).toISOString(),
    value: Math.max(0, totalMps + Math.sin(i / 10) * totalMps * 0.15 + (Math.random() - 0.5) * totalMps * 0.1),
  }));
}

// --- Consumer lag history ---
export function getConsumerLagHistory(topicName: string, points: number = 60): TimeSeriesPoint[] {
  const topic = KAFKA_TOPICS.find(t => t.name === topicName);
  const baseLag = topic?.lag ?? 10;
  const now = Date.now();

  return Array.from({ length: points }, (_, i) => ({
    timestamp: new Date(now - (points - i) * 60000).toISOString(),
    value: Math.max(0, baseLag + Math.sin(i / 6) * baseLag * 0.5 + Math.random() * 5),
  }));
}

// --- Message generation helpers ---
function generateMessageKey(category: string): string {
  const regions = ['us-west', 'us-east', 'us-central', 'us-south', 'pacific', 'atlantic'];
  const region = regions[Math.floor(Math.random() * regions.length)];
  return `${category}:${region}:${Date.now()}`;
}

function generateMessageValue(category: string): Record<string, unknown> {
  switch (category) {
    case 'seismic_events':
      return {
        magnitude: +(Math.random() * 4 + 0.5).toFixed(1),
        depth_km: +(Math.random() * 30 + 1).toFixed(1),
        lat: +(Math.random() * 40 + 20).toFixed(4),
        lng: +(Math.random() * -60 - 70).toFixed(4),
        source: 'USGS',
        processing_time_ms: Math.floor(Math.random() * 50 + 10),
      };
    case 'weather_data':
      return {
        temperature_f: +(Math.random() * 60 + 30).toFixed(1),
        humidity_pct: +(Math.random() * 60 + 20).toFixed(1),
        wind_speed_mph: +(Math.random() * 40).toFixed(1),
        pressure_mb: +(Math.random() * 30 + 1000).toFixed(1),
        station_id: `NWS-${Math.floor(Math.random() * 9000 + 1000)}`,
      };
    case 'air_quality':
      return {
        aqi: Math.floor(Math.random() * 200),
        pm25: +(Math.random() * 50).toFixed(1),
        pm10: +(Math.random() * 100).toFixed(1),
        o3_ppb: +(Math.random() * 80).toFixed(1),
        no2_ppb: +(Math.random() * 40).toFixed(1),
        monitor_id: `AQS-${Math.floor(Math.random() * 9000 + 1000)}`,
      };
    case 'water_levels':
      return {
        gauge_height_ft: +(Math.random() * 20 + 5).toFixed(2),
        discharge_cfs: Math.floor(Math.random() * 50000 + 1000),
        water_temp_f: +(Math.random() * 30 + 45).toFixed(1),
        site_no: `${Math.floor(Math.random() * 90000000 + 10000000)}`,
      };
    case 'fire_detections':
      return {
        confidence: +(Math.random() * 40 + 60).toFixed(0),
        brightness_k: +(Math.random() * 100 + 300).toFixed(1),
        frp_mw: +(Math.random() * 50 + 5).toFixed(1),
        satellite: Math.random() > 0.5 ? 'MODIS' : 'VIIRS',
        scan_area_km2: +(Math.random() * 2 + 0.5).toFixed(2),
      };
    default:
      return {
        value: +(Math.random() * 100).toFixed(2),
        unit: 'unknown',
        sensor_id: `SEN-${Math.floor(Math.random() * 9000 + 1000)}`,
      };
  }
}

export { KAFKA_TOPICS };
