// ============================================================================
// Alert Management Service
// Combines alerts from public APIs, AI models, and Spark analytics
// ============================================================================

import type { Alert } from '../types';

// --- Internal/AI-generated alerts ---
const INTERNAL_ALERTS: Alert[] = [
  {
    id: 'alert-001',
    title: 'Structural Anomaly Detected - I-40 Bridge',
    description: 'Spark anomaly detection model identified accelerating strain rate in main beam sensor. Rate increased 340% over last 72 hours. Immediate inspection recommended.',
    severity: 'emergency',
    status: 'active',
    source: 'spark_analytics',
    location: { lat: 35.1495, lng: -90.0490 },
    assetId: 'asset-007',
    timestamp: new Date(Date.now() - 30 * 60000).toISOString(),
    tags: ['structural', 'bridge', 'ai-detected', 'critical-infrastructure'],
  },
  {
    id: 'alert-002',
    title: 'Gas Leak Probability Elevated - Keystone NE-12',
    description: 'AI model detected anomalous acoustic pattern combined with 0.2ppm methane reading. Pattern matches historical leak precursors with 78% confidence.',
    severity: 'critical',
    status: 'active',
    source: 'ai_model',
    location: { lat: 42.8, lng: -97.4 },
    assetId: 'asset-003',
    sensorId: 'sen-021',
    timestamp: new Date(Date.now() - 2 * 3600000).toISOString(),
    tags: ['pipeline', 'gas-leak', 'ai-detected', 'hazmat'],
  },
  {
    id: 'alert-003',
    title: 'Wildfire Risk Extreme - Pacific Crest Corridor',
    description: 'Spark trend analysis: soil moisture at 18.5% (critical threshold: 20%), combined with 3-day high wind forecast. Fire risk model score: 94/100.',
    severity: 'critical',
    status: 'active',
    source: 'spark_analytics',
    location: { lat: 44.0, lng: -121.5 },
    assetId: 'asset-009',
    timestamp: new Date(Date.now() - 4 * 3600000).toISOString(),
    tags: ['wildfire', 'forest', 'ai-detected', 'climate'],
  },
  {
    id: 'alert-004',
    title: 'Oroville Dam - Seepage Rate Increasing',
    description: 'Water seepage monitoring indicates 15% increase over past week. While within normal bounds, trend warrants monitoring given dam history.',
    severity: 'warning',
    status: 'active',
    source: 'sensor',
    location: { lat: 39.5378, lng: -121.4847 },
    assetId: 'asset-006',
    timestamp: new Date(Date.now() - 6 * 3600000).toISOString(),
    tags: ['dam', 'seepage', 'water', 'infrastructure'],
  },
  {
    id: 'alert-005',
    title: 'Power Line Sag Exceeds Threshold - PG&E Line 47',
    description: 'Conductor sag measured at 12.8ft, exceeding 12ft safety threshold. Likely due to thermal expansion from high ambient temperature and line loading.',
    severity: 'warning',
    status: 'acknowledged',
    source: 'sensor',
    location: { lat: 38.5816, lng: -121.4944 },
    assetId: 'asset-002',
    sensorId: 'sen-011',
    timestamp: new Date(Date.now() - 8 * 3600000).toISOString(),
    acknowledgedAt: new Date(Date.now() - 7 * 3600000).toISOString(),
    tags: ['power', 'utility', 'sag', 'thermal'],
  },
  {
    id: 'alert-006',
    title: 'Central Valley Subsidence Accelerating',
    description: 'Satellite InSAR analysis by Spark batch job shows land subsidence rate increased to 2.4 inches/year. Aquifer depletion contributing to irrigation infrastructure damage.',
    severity: 'warning',
    status: 'active',
    source: 'spark_analytics',
    location: { lat: 36.7783, lng: -119.4179 },
    assetId: 'asset-010',
    timestamp: new Date(Date.now() - 12 * 3600000).toISOString(),
    tags: ['subsidence', 'agriculture', 'groundwater', 'satellite'],
  },
  {
    id: 'alert-007',
    title: 'Lake Mead Water Level Below Critical Threshold',
    description: 'Water level at 1065.8ft MSL, approaching the 1050ft dead pool elevation. Colorado River allocation triggers imminent.',
    severity: 'warning',
    status: 'active',
    source: 'sensor',
    location: { lat: 36.1453, lng: -114.3896 },
    sensorId: 'sen-071',
    timestamp: new Date(Date.now() - 24 * 3600000).toISOString(),
    tags: ['water', 'drought', 'reservoir', 'colorado-river'],
  },
  {
    id: 'alert-008',
    title: 'Air Quality Unhealthy - Houston Industrial Zone',
    description: 'AQI reading of 95 with upward trend. PM2.5 elevated from refinery emissions combined with stagnant air mass.',
    severity: 'info',
    status: 'active',
    source: 'public_api',
    location: { lat: 29.7604, lng: -95.3698 },
    sensorId: 'sen-042',
    timestamp: new Date(Date.now() - 3 * 3600000).toISOString(),
    tags: ['air-quality', 'industrial', 'pm25', 'epa'],
  },
  {
    id: 'alert-009',
    title: 'Sensor Offline - Sequoia NF Temperature',
    description: 'Temperature sensor at Sequoia National Forest has been offline for 1 hour. Last known reading: N/A. Possible power failure or communication issue.',
    severity: 'info',
    status: 'active',
    source: 'sensor',
    location: { lat: 36.4864, lng: -118.5658 },
    sensorId: 'sen-062',
    timestamp: new Date(Date.now() - 1 * 3600000).toISOString(),
    tags: ['sensor', 'offline', 'maintenance'],
  },
  {
    id: 'alert-010',
    title: 'Spark Job Failed - Traffic-Infrastructure Impact',
    description: 'Executor memory overflow on heavy traffic partition. Job spark-008 failed at stage 5/6. Auto-retry scheduled with increased memory allocation.',
    severity: 'info',
    status: 'active',
    source: 'spark_analytics',
    timestamp: new Date(Date.now() - 1 * 3600000).toISOString(),
    tags: ['spark', 'job-failure', 'system'],
  },
];

export function getAlerts(): Alert[] {
  return INTERNAL_ALERTS;
}

export function getActiveAlerts(): Alert[] {
  return INTERNAL_ALERTS.filter(a => a.status === 'active');
}

export function getAlertsBySeverity(severity: string): Alert[] {
  return INTERNAL_ALERTS.filter(a => a.severity === severity);
}

export function getAlertCounts(): Record<string, number> {
  return INTERNAL_ALERTS.reduce((acc, alert) => {
    acc[alert.severity] = (acc[alert.severity] || 0) + 1;
    return acc;
  }, {} as Record<string, number>);
}

export function getCriticalAndEmergencyAlerts(): Alert[] {
  return INTERNAL_ALERTS
    .filter(a => (a.severity === 'critical' || a.severity === 'emergency') && a.status === 'active')
    .sort((a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime());
}
