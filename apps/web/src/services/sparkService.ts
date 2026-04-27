// ============================================================================
// Apache Spark Large-Scale Data Processing Service
// Simulates Spark cluster metrics, streaming jobs, and analytics results
// In production, connects to Spark UI REST API or Livy server
// ============================================================================

import type {
  SparkJob,
  SparkClusterStats,
  TimeSeriesPoint,
  TimeSeriesData,
} from '../types';

// --- Active and Recent Spark Jobs ---
const SPARK_JOBS: SparkJob[] = [
  {
    id: 'spark-001',
    name: 'Seismic Anomaly Detection',
    type: 'anomaly_detection',
    status: 'running',
    progress: 78,
    startedAt: new Date(Date.now() - 45 * 60000).toISOString(),
    inputRecords: 2_340_000,
    outputRecords: 156,
    stages: 8,
    completedStages: 6,
    executors: 12,
    memoryUsed: 8_589_934_592,
    shuffleRead: 2_147_483_648,
    shuffleWrite: 536_870_912,
    inputTopics: ['physical-obs.seismic-events'],
    description: 'Real-time seismic pattern analysis using ML models trained on USGS historical data. Detects P-wave/S-wave anomalies and correlates with tectonic plate activity.',
  },
  {
    id: 'spark-002',
    name: 'Wildfire Spread Prediction',
    type: 'ml_inference',
    status: 'running',
    progress: 42,
    startedAt: new Date(Date.now() - 22 * 60000).toISOString(),
    inputRecords: 890_000,
    outputRecords: 34,
    stages: 12,
    completedStages: 5,
    executors: 24,
    memoryUsed: 17_179_869_184,
    shuffleRead: 4_294_967_296,
    shuffleWrite: 1_073_741_824,
    inputTopics: ['physical-obs.fire-detections', 'physical-obs.weather-data', 'physical-obs.satellite-imagery'],
    description: 'Combines NASA FIRMS hotspot data with NOAA weather patterns and Sentinel-2 vegetation indices to predict fire spread trajectories up to 72 hours ahead.',
  },
  {
    id: 'spark-003',
    name: 'Air Quality Index Aggregation',
    type: 'stream_processing',
    status: 'running',
    progress: 95,
    startedAt: new Date(Date.now() - 60 * 60000).toISOString(),
    inputRecords: 5_670_000,
    outputRecords: 445_000,
    stages: 4,
    completedStages: 4,
    executors: 8,
    memoryUsed: 4_294_967_296,
    shuffleRead: 1_073_741_824,
    shuffleWrite: 268_435_456,
    inputTopics: ['physical-obs.air-quality', 'physical-obs.global-air-quality'],
    description: 'Continuous streaming aggregation of EPA AirNow and OpenAQ data. Computes rolling AQI averages, identifies pollution hotspots, and generates county-level air quality maps.',
  },
  {
    id: 'spark-004',
    name: 'Flood Risk Scoring',
    type: 'risk_scoring',
    status: 'running',
    progress: 61,
    startedAt: new Date(Date.now() - 35 * 60000).toISOString(),
    inputRecords: 1_230_000,
    outputRecords: 8_900,
    stages: 6,
    completedStages: 4,
    executors: 16,
    memoryUsed: 12_884_901_888,
    shuffleRead: 3_221_225_472,
    shuffleWrite: 805_306_368,
    inputTopics: ['physical-obs.water-levels', 'physical-obs.weather-data', 'physical-obs.soil-conditions'],
    description: 'Correlates USGS stream gauge data with NOAA precipitation forecasts and soil saturation levels to generate real-time flood risk scores for watersheds.',
  },
  {
    id: 'spark-005',
    name: 'Infrastructure Degradation Analysis',
    type: 'batch_analysis',
    status: 'completed',
    progress: 100,
    startedAt: new Date(Date.now() - 180 * 60000).toISOString(),
    completedAt: new Date(Date.now() - 120 * 60000).toISOString(),
    duration: 3_600_000,
    inputRecords: 12_000_000,
    outputRecords: 2_340,
    stages: 16,
    completedStages: 16,
    executors: 32,
    memoryUsed: 0,
    shuffleRead: 8_589_934_592,
    shuffleWrite: 2_147_483_648,
    inputTopics: ['physical-obs.structural-health', 'physical-obs.satellite-imagery'],
    description: 'Batch analysis of structural health sensor data combined with satellite imagery to detect micro-fractures, corrosion patterns, and load-bearing degradation in bridges and dams.',
  },
  {
    id: 'spark-006',
    name: 'Geospatial Event Correlation',
    type: 'geospatial_correlation',
    status: 'running',
    progress: 33,
    startedAt: new Date(Date.now() - 15 * 60000).toISOString(),
    inputRecords: 3_450_000,
    outputRecords: 789,
    stages: 10,
    completedStages: 3,
    executors: 20,
    memoryUsed: 10_737_418_240,
    shuffleRead: 2_684_354_560,
    shuffleWrite: 671_088_640,
    inputTopics: ['physical-obs.seismic-events', 'physical-obs.water-levels', 'physical-obs.gas-emissions'],
    description: 'Multi-source geospatial correlation engine that identifies potential cascading failures - e.g., seismic events affecting pipeline integrity near water systems.',
  },
  {
    id: 'spark-007',
    name: 'Climate Trend Analysis',
    type: 'trend_analysis',
    status: 'completed',
    progress: 100,
    startedAt: new Date(Date.now() - 240 * 60000).toISOString(),
    completedAt: new Date(Date.now() - 90 * 60000).toISOString(),
    duration: 9_000_000,
    inputRecords: 45_000_000,
    outputRecords: 12_000,
    stages: 20,
    completedStages: 20,
    executors: 48,
    memoryUsed: 0,
    shuffleRead: 17_179_869_184,
    shuffleWrite: 4_294_967_296,
    inputTopics: ['physical-obs.climate-data', 'physical-obs.weather-data'],
    description: 'Long-term climate trend analysis processing 10 years of NOAA historical data to identify accelerating patterns in temperature extremes, precipitation anomalies, and drought cycles.',
  },
  {
    id: 'spark-008',
    name: 'Traffic-Infrastructure Impact',
    type: 'stream_processing',
    status: 'failed',
    progress: 67,
    startedAt: new Date(Date.now() - 90 * 60000).toISOString(),
    completedAt: new Date(Date.now() - 60 * 60000).toISOString(),
    duration: 1_800_000,
    inputRecords: 8_900_000,
    outputRecords: 0,
    stages: 6,
    completedStages: 4,
    executors: 12,
    memoryUsed: 0,
    shuffleRead: 2_147_483_648,
    shuffleWrite: 536_870_912,
    inputTopics: ['physical-obs.traffic-flow', 'physical-obs.structural-health'],
    description: 'Correlates traffic load patterns with bridge structural health sensors to estimate infrastructure fatigue. Failed due to executor memory overflow on heavy traffic partition.',
  },
];

// --- Get Spark jobs with real-time progress updates ---
export function getSparkJobs(): SparkJob[] {
  return SPARK_JOBS.map(job => {
    if (job.status === 'running') {
      return {
        ...job,
        progress: Math.min(99, job.progress + Math.floor(Math.random() * 3)),
        outputRecords: job.outputRecords + Math.floor(Math.random() * 100),
        memoryUsed: job.memoryUsed + Math.floor(Math.random() * 100_000_000 - 50_000_000),
      };
    }
    return job;
  });
}

// --- Spark Cluster Stats ---
export function getSparkClusterStats(): SparkClusterStats {
  const jobs = getSparkJobs();
  const runningJobs = jobs.filter(j => j.status === 'running');
  return {
    activeJobs: runningJobs.length,
    completedJobs: jobs.filter(j => j.status === 'completed').length,
    failedJobs: jobs.filter(j => j.status === 'failed').length,
    totalExecutors: 64,
    activeExecutors: runningJobs.reduce((sum, j) => sum + j.executors, 0),
    totalMemory: 274_877_906_944, // 256 GB
    usedMemory: runningJobs.reduce((sum, j) => sum + j.memoryUsed, 0),
    totalCores: 256,
    activeCores: runningJobs.reduce((sum, j) => sum + j.executors * 4, 0),
    streamingBatches: 1_247,
    avgBatchDuration: 2_340,
  };
}

// --- Analytics Results (processed by Spark) ---

export function getAirQualityTrends(): TimeSeriesData {
  const now = Date.now();
  return {
    metric: 'Average AQI (National)',
    unit: 'AQI',
    points: Array.from({ length: 90 }, (_, i) => ({
      timestamp: new Date(now - (90 - i) * 86400000).toISOString(),
      value: Math.max(20, 55 + Math.sin(i / 15) * 25 + (Math.random() - 0.5) * 15 + (i > 60 ? 10 : 0)),
    })),
  };
}

export function getSeismicActivityTrends(): TimeSeriesData {
  const now = Date.now();
  return {
    metric: 'Daily Earthquake Count (M1.0+)',
    unit: 'events',
    points: Array.from({ length: 30 }, (_, i) => ({
      timestamp: new Date(now - (30 - i) * 86400000).toISOString(),
      value: Math.max(5, 35 + Math.sin(i / 5) * 15 + (Math.random() - 0.5) * 10),
    })),
  };
}

export function getWaterLevelTrends(): TimeSeriesData {
  const now = Date.now();
  return {
    metric: 'Mississippi River Gauge (St. Louis)',
    unit: 'feet',
    points: Array.from({ length: 168 }, (_, i) => ({
      timestamp: new Date(now - (168 - i) * 3600000).toISOString(),
      value: 18 + Math.sin(i / 24) * 3 + Math.sin(i / 72) * 5 + (Math.random() - 0.5) * 0.5,
    })),
  };
}

export function getFireDetectionTrends(): TimeSeriesData {
  const now = Date.now();
  return {
    metric: 'Active Fire Hotspots (US)',
    unit: 'detections',
    points: Array.from({ length: 30 }, (_, i) => ({
      timestamp: new Date(now - (30 - i) * 86400000).toISOString(),
      value: Math.max(10, 120 + Math.sin(i / 7) * 60 + (Math.random() - 0.5) * 40 + (i > 20 ? 30 : 0)),
    })),
  };
}

export function getInfrastructureRiskDistribution(): { category: string; count: number; avgRisk: number }[] {
  return [
    { category: 'Bridges', count: 614_387, avgRisk: 34 },
    { category: 'Dams', count: 91_457, avgRisk: 28 },
    { category: 'Pipelines', count: 2_700_000, avgRisk: 41 },
    { category: 'Power Lines', count: 640_000, avgRisk: 22 },
    { category: 'Waterways', count: 25_000, avgRisk: 37 },
    { category: 'Roads', count: 4_190_000, avgRisk: 31 },
  ];
}

// --- Spark Processing Throughput History ---
export function getSparkThroughputHistory(points: number = 60): TimeSeriesPoint[] {
  const now = Date.now();
  return Array.from({ length: points }, (_, i) => ({
    timestamp: new Date(now - (points - i) * 60000).toISOString(),
    value: Math.max(50000, 450000 + Math.sin(i / 12) * 150000 + (Math.random() - 0.5) * 80000),
  }));
}

// --- Spark Memory Usage History ---
export function getSparkMemoryHistory(points: number = 60): TimeSeriesPoint[] {
  const totalGB = 256;
  const now = Date.now();
  return Array.from({ length: points }, (_, i) => ({
    timestamp: new Date(now - (points - i) * 60000).toISOString(),
    value: Math.max(40, Math.min(95, 68 + Math.sin(i / 8) * 15 + (Math.random() - 0.5) * 8)),
  })).map(p => ({ ...p, value: +(p.value * totalGB / 100).toFixed(1) }));
}
