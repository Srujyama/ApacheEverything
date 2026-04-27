// ============================================================================
// Sensor & Infrastructure Asset Service
// Manages sensor network and infrastructure asset inventory
// ============================================================================

import type { Sensor, InfrastructureAsset } from '../types';

// --- Simulated Sensor Network ---
const SENSORS: Sensor[] = [
  // Bridge sensors
  { id: 'sen-001', name: 'Golden Gate Bridge - N Tower Strain', type: 'structural_strain', location: { lat: 37.8199, lng: -122.4783 }, status: 'online', lastReading: 0.0023, unit: 'mm/m', lastUpdated: new Date().toISOString(), assetId: 'asset-001' },
  { id: 'sen-002', name: 'Golden Gate Bridge - S Tower Strain', type: 'structural_strain', location: { lat: 37.8080, lng: -122.4745 }, status: 'online', lastReading: 0.0019, unit: 'mm/m', lastUpdated: new Date().toISOString(), assetId: 'asset-001' },
  { id: 'sen-003', name: 'Golden Gate Bridge - Wind Sensor', type: 'wind_speed', location: { lat: 37.8150, lng: -122.4770 }, status: 'online', lastReading: 22.4, unit: 'mph', lastUpdated: new Date().toISOString(), assetId: 'asset-001' },
  { id: 'sen-004', name: 'Golden Gate Bridge - Seismic', type: 'seismic', location: { lat: 37.8150, lng: -122.4770 }, status: 'online', lastReading: 0.001, unit: 'g', lastUpdated: new Date().toISOString(), assetId: 'asset-001' },

  // Power infrastructure
  { id: 'sen-010', name: 'PG&E Line 47 - Temp Monitor', type: 'temperature', location: { lat: 38.5816, lng: -121.4944 }, status: 'online', lastReading: 142.3, unit: 'F', lastUpdated: new Date().toISOString(), assetId: 'asset-002' },
  { id: 'sen-011', name: 'PG&E Line 47 - Sag Monitor', type: 'structural_strain', location: { lat: 38.5820, lng: -121.4950 }, status: 'degraded', lastReading: 12.8, unit: 'ft', lastUpdated: new Date().toISOString(), assetId: 'asset-002' },

  // Pipeline sensors
  { id: 'sen-020', name: 'Keystone XL - Pressure Monitor A', type: 'gas_detector', location: { lat: 42.8, lng: -97.4 }, status: 'online', lastReading: 856, unit: 'PSI', lastUpdated: new Date().toISOString(), assetId: 'asset-003' },
  { id: 'sen-021', name: 'Keystone XL - Gas Leak Detector', type: 'gas_detector', location: { lat: 42.81, lng: -97.38 }, status: 'online', lastReading: 0.2, unit: 'ppm', lastUpdated: new Date().toISOString(), assetId: 'asset-003' },
  { id: 'sen-022', name: 'Keystone XL - Acoustic Sensor', type: 'acoustic', location: { lat: 42.79, lng: -97.42 }, status: 'online', lastReading: 34, unit: 'dB', lastUpdated: new Date().toISOString(), assetId: 'asset-003' },

  // Dam sensors
  { id: 'sen-030', name: 'Hoover Dam - Water Level', type: 'water_level', location: { lat: 36.0161, lng: -114.7377 }, status: 'online', lastReading: 1043.2, unit: 'ft MSL', lastUpdated: new Date().toISOString(), assetId: 'asset-004' },
  { id: 'sen-031', name: 'Hoover Dam - Seepage Monitor', type: 'water_level', location: { lat: 36.0155, lng: -114.7370 }, status: 'online', lastReading: 0.45, unit: 'GPM', lastUpdated: new Date().toISOString(), assetId: 'asset-004' },
  { id: 'sen-032', name: 'Hoover Dam - Structural Strain', type: 'structural_strain', location: { lat: 36.0160, lng: -114.7375 }, status: 'online', lastReading: 0.0015, unit: 'mm/m', lastUpdated: new Date().toISOString(), assetId: 'asset-004' },

  // Air quality
  { id: 'sen-040', name: 'LA Downtown AQI Monitor', type: 'air_quality', location: { lat: 34.0522, lng: -118.2437 }, status: 'online', lastReading: 78, unit: 'AQI', lastUpdated: new Date().toISOString() },
  { id: 'sen-041', name: 'SF Bay Area AQI Monitor', type: 'air_quality', location: { lat: 37.7749, lng: -122.4194 }, status: 'online', lastReading: 42, unit: 'AQI', lastUpdated: new Date().toISOString() },
  { id: 'sen-042', name: 'Houston Industrial AQI', type: 'air_quality', location: { lat: 29.7604, lng: -95.3698 }, status: 'online', lastReading: 95, unit: 'AQI', lastUpdated: new Date().toISOString() },

  // Weather stations
  { id: 'sen-050', name: 'NOAA Station - Miami', type: 'temperature', location: { lat: 25.7617, lng: -80.1918 }, status: 'online', lastReading: 89.2, unit: 'F', lastUpdated: new Date().toISOString() },
  { id: 'sen-051', name: 'NOAA Station - Denver', type: 'humidity', location: { lat: 39.7392, lng: -104.9903 }, status: 'online', lastReading: 32, unit: '%', lastUpdated: new Date().toISOString() },
  { id: 'sen-052', name: 'NOAA Station - Seattle', type: 'wind_speed', location: { lat: 47.6062, lng: -122.3321 }, status: 'online', lastReading: 15.7, unit: 'mph', lastUpdated: new Date().toISOString() },

  // Forest/wildfire
  { id: 'sen-060', name: 'Yellowstone Fire Watch Cam', type: 'camera', location: { lat: 44.4280, lng: -110.5885 }, status: 'online', lastReading: 1, unit: 'active', lastUpdated: new Date().toISOString() },
  { id: 'sen-061', name: 'Yosemite Soil Moisture', type: 'soil_moisture', location: { lat: 37.8651, lng: -119.5383 }, status: 'online', lastReading: 18.5, unit: '%', lastUpdated: new Date().toISOString() },
  { id: 'sen-062', name: 'Sequoia NF Temperature', type: 'temperature', location: { lat: 36.4864, lng: -118.5658 }, status: 'offline', lastReading: 0, unit: 'F', lastUpdated: new Date(Date.now() - 3600000).toISOString() },

  // Water
  { id: 'sen-070', name: 'Mississippi River - Memphis Gauge', type: 'water_level', location: { lat: 35.1495, lng: -90.0490 }, status: 'online', lastReading: 24.3, unit: 'ft', lastUpdated: new Date().toISOString() },
  { id: 'sen-071', name: 'Colorado River - Lake Mead', type: 'water_level', location: { lat: 36.1453, lng: -114.3896 }, status: 'online', lastReading: 1065.8, unit: 'ft MSL', lastUpdated: new Date().toISOString() },
  { id: 'sen-072', name: 'Columbia River - Portland', type: 'water_level', location: { lat: 45.5152, lng: -122.6784 }, status: 'online', lastReading: 8.7, unit: 'ft', lastUpdated: new Date().toISOString() },
];

// --- Infrastructure Assets ---
const ASSETS: InfrastructureAsset[] = [
  {
    id: 'asset-001',
    name: 'Golden Gate Bridge',
    category: 'bridge',
    location: { lat: 37.8199, lng: -122.4783 },
    condition: 'good',
    sensors: ['sen-001', 'sen-002', 'sen-003', 'sen-004'],
    lastInspection: '2025-11-15',
    nextInspection: '2026-05-15',
    riskScore: 18,
    metadata: { built: 1937, span_ft: 4200, material: 'Steel', daily_traffic: 40000 },
  },
  {
    id: 'asset-002',
    name: 'PG&E Transmission Line 47',
    category: 'power_line',
    location: { lat: 38.5816, lng: -121.4944 },
    condition: 'fair',
    sensors: ['sen-010', 'sen-011'],
    lastInspection: '2025-09-01',
    nextInspection: '2026-03-01',
    riskScore: 42,
    metadata: { voltage_kv: 230, length_miles: 87, age_years: 34, material: 'ACSR Conductor' },
  },
  {
    id: 'asset-003',
    name: 'Keystone Pipeline Segment NE-12',
    category: 'pipeline',
    location: { lat: 42.8, lng: -97.4 },
    condition: 'good',
    sensors: ['sen-020', 'sen-021', 'sen-022'],
    lastInspection: '2026-01-10',
    nextInspection: '2026-07-10',
    riskScore: 25,
    metadata: { diameter_in: 36, pressure_psi: 856, material: 'Carbon Steel', content: 'Crude Oil' },
  },
  {
    id: 'asset-004',
    name: 'Hoover Dam',
    category: 'dam',
    location: { lat: 36.0161, lng: -114.7377 },
    condition: 'good',
    sensors: ['sen-030', 'sen-031', 'sen-032'],
    lastInspection: '2025-12-01',
    nextInspection: '2026-06-01',
    riskScore: 12,
    metadata: { built: 1936, height_ft: 726, capacity_acre_ft: 26200000, power_mw: 2080 },
  },
  {
    id: 'asset-005',
    name: 'Brooklyn Bridge',
    category: 'bridge',
    location: { lat: 40.7061, lng: -73.9969 },
    condition: 'fair',
    sensors: [],
    lastInspection: '2025-08-20',
    nextInspection: '2026-02-20',
    riskScore: 38,
    metadata: { built: 1883, span_ft: 1595, material: 'Steel/Stone', daily_traffic: 120000 },
  },
  {
    id: 'asset-006',
    name: 'Oroville Dam',
    category: 'dam',
    location: { lat: 39.5378, lng: -121.4847 },
    condition: 'degraded',
    sensors: [],
    lastInspection: '2025-10-15',
    nextInspection: '2026-01-15',
    riskScore: 67,
    metadata: { built: 1968, height_ft: 770, capacity_acre_ft: 3537577, spillway_status: 'Repaired' },
  },
  {
    id: 'asset-007',
    name: 'I-40 Mississippi River Bridge',
    category: 'bridge',
    location: { lat: 35.1495, lng: -90.0490 },
    condition: 'critical',
    sensors: [],
    lastInspection: '2026-01-05',
    nextInspection: '2026-04-05',
    riskScore: 82,
    metadata: { built: 1973, span_ft: 2500, material: 'Steel', daily_traffic: 53000, note: 'Fracture detected in main beam' },
  },
  {
    id: 'asset-008',
    name: 'Trans-Alaska Pipeline Segment AK-3',
    category: 'pipeline',
    location: { lat: 64.2008, lng: -149.4937 },
    condition: 'good',
    sensors: [],
    lastInspection: '2025-07-20',
    nextInspection: '2026-01-20',
    riskScore: 30,
    metadata: { diameter_in: 48, length_miles: 800, material: 'Carbon Steel', content: 'Crude Oil' },
  },
  {
    id: 'asset-009',
    name: 'Pacific Crest Trail Forest Corridor',
    category: 'forest',
    location: { lat: 44.0, lng: -121.5 },
    condition: 'fair',
    sensors: [],
    lastInspection: '2025-09-01',
    nextInspection: '2026-06-01',
    riskScore: 55,
    metadata: { area_acres: 250000, fire_risk: 'High', drought_index: 'Severe', tree_mortality_pct: 12 },
  },
  {
    id: 'asset-010',
    name: 'Central Valley Irrigation District',
    category: 'cropland',
    location: { lat: 36.7783, lng: -119.4179 },
    condition: 'degraded',
    sensors: [],
    lastInspection: '2025-11-01',
    nextInspection: '2026-05-01',
    riskScore: 61,
    metadata: { area_acres: 500000, crop_type: 'Mixed Agriculture', water_source: 'Groundwater + Canal', subsidence_rate_in_yr: 2.4 },
  },
];

// --- Service Functions ---
export function getSensors(): Sensor[] {
  return SENSORS.map(s => ({
    ...s,
    lastReading: s.status === 'online'
      ? +(s.lastReading + (Math.random() - 0.5) * s.lastReading * 0.05).toFixed(4)
      : s.lastReading,
    lastUpdated: s.status === 'online' ? new Date().toISOString() : s.lastUpdated,
  }));
}

export function getSensorById(id: string): Sensor | undefined {
  return SENSORS.find(s => s.id === id);
}

export function getAssets(): InfrastructureAsset[] {
  return ASSETS;
}

export function getAssetById(id: string): InfrastructureAsset | undefined {
  return ASSETS.find(a => a.id === id);
}

export function getAssetsByCriticalRisk(): InfrastructureAsset[] {
  return ASSETS.filter(a => a.riskScore >= 60).sort((a, b) => b.riskScore - a.riskScore);
}

export function getSensorsByStatus(): { online: number; offline: number; degraded: number; maintenance: number } {
  return SENSORS.reduce(
    (acc, s) => {
      acc[s.status]++;
      return acc;
    },
    { online: 0, offline: 0, degraded: 0, maintenance: 0 }
  );
}
