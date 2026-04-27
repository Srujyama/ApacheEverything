import { useState, useEffect } from 'react';
import { MapContainer, TileLayer, CircleMarker, Popup, LayersControl } from 'react-leaflet';
import { Layers, AlertTriangle, Radio as RadioIcon, Building2 } from 'lucide-react';
import { fetchRecentEarthquakes, fetchSignificantEarthquakes } from '../services/publicDataSources';
import { getSensors, getAssets } from '../services/sensorService';
import { getCriticalAndEmergencyAlerts } from '../services/alertService';
import type { MapMarker, Sensor, InfrastructureAsset } from '../types';
import { severityColor, conditionColor } from '../utils/format';
import 'leaflet/dist/leaflet.css';
import './LiveMap.css';

export default function LiveMap() {
  const [earthquakes, setEarthquakes] = useState<MapMarker[]>([]);
  const [significantQuakes, setSignificantQuakes] = useState<MapMarker[]>([]);
  const [showLayer, setShowLayer] = useState({
    sensors: true,
    assets: true,
    earthquakes: true,
    alerts: true,
  });

  const sensors = getSensors();
  const assets = getAssets();
  const criticalAlerts = getCriticalAndEmergencyAlerts();

  useEffect(() => {
    fetchRecentEarthquakes().then(setEarthquakes);
    fetchSignificantEarthquakes().then(setSignificantQuakes);
  }, []);

  const allEarthquakes = [...significantQuakes, ...earthquakes];

  function getSensorColor(sensor: Sensor): string {
    if (sensor.status === 'offline') return '#ef4444';
    if (sensor.status === 'degraded') return '#eab308';
    return '#22c55e';
  }

  function getAssetColor(asset: InfrastructureAsset): string {
    if (asset.riskScore >= 60) return '#ef4444';
    if (asset.riskScore >= 40) return '#f97316';
    if (asset.riskScore >= 20) return '#eab308';
    return '#22c55e';
  }

  function getQuakeRadius(marker: MapMarker): number {
    const mag = (marker.details.magnitude as number) || 1;
    return Math.max(4, mag * 4);
  }

  return (
    <div className="live-map-page">
      <div className="page-header">
        <div>
          <h1>Live Infrastructure Map</h1>
          <p className="page-subtitle">
            Real-time geospatial view of sensors, assets, and events from USGS, NOAA, and NASA
          </p>
        </div>
      </div>

      <div className="map-controls">
        <button
          className={`map-layer-btn ${showLayer.sensors ? 'active' : ''}`}
          onClick={() => setShowLayer(s => ({ ...s, sensors: !s.sensors }))}
        >
          <RadioIcon size={14} />
          Sensors ({sensors.length})
        </button>
        <button
          className={`map-layer-btn ${showLayer.assets ? 'active' : ''}`}
          onClick={() => setShowLayer(s => ({ ...s, assets: !s.assets }))}
        >
          <Building2 size={14} />
          Assets ({assets.length})
        </button>
        <button
          className={`map-layer-btn ${showLayer.earthquakes ? 'active' : ''}`}
          onClick={() => setShowLayer(s => ({ ...s, earthquakes: !s.earthquakes }))}
        >
          <Layers size={14} />
          Earthquakes ({allEarthquakes.length})
        </button>
        <button
          className={`map-layer-btn ${showLayer.alerts ? 'active' : ''}`}
          onClick={() => setShowLayer(s => ({ ...s, alerts: !s.alerts }))}
        >
          <AlertTriangle size={14} />
          Alerts ({criticalAlerts.filter(a => a.location).length})
        </button>
      </div>

      <div className="map-container">
        <MapContainer
          center={[39.8283, -98.5795]}
          zoom={4}
          scrollWheelZoom={true}
          style={{ height: '100%', width: '100%' }}
        >
          <LayersControl position="topright">
            <LayersControl.BaseLayer checked name="Dark">
              <TileLayer
                attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OSM</a> &copy; <a href="https://carto.com/">CARTO</a>'
                url="https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png"
              />
            </LayersControl.BaseLayer>
            <LayersControl.BaseLayer name="Satellite">
              <TileLayer
                attribution='&copy; Esri'
                url="https://server.arcgisonline.com/ArcGIS/rest/services/World_Imagery/MapServer/tile/{z}/{y}/{x}"
              />
            </LayersControl.BaseLayer>
            <LayersControl.BaseLayer name="Terrain">
              <TileLayer
                attribution='&copy; <a href="https://www.openstreetmap.org/copyright">OSM</a>'
                url="https://{s}.tile.opentopomap.org/{z}/{x}/{y}.png"
              />
            </LayersControl.BaseLayer>
          </LayersControl>

          {/* Sensors */}
          {showLayer.sensors && sensors.map(sensor => (
            <CircleMarker
              key={sensor.id}
              center={[sensor.location.lat, sensor.location.lng]}
              radius={6}
              fillColor={getSensorColor(sensor)}
              color={getSensorColor(sensor)}
              weight={2}
              opacity={0.8}
              fillOpacity={0.5}
            >
              <Popup>
                <div className="map-popup">
                  <h4>{sensor.name}</h4>
                  <div className="map-popup-row">
                    <span>Type:</span><span>{sensor.type}</span>
                  </div>
                  <div className="map-popup-row">
                    <span>Status:</span>
                    <span style={{ color: getSensorColor(sensor) }}>{sensor.status}</span>
                  </div>
                  <div className="map-popup-row">
                    <span>Reading:</span><span>{sensor.lastReading} {sensor.unit}</span>
                  </div>
                </div>
              </Popup>
            </CircleMarker>
          ))}

          {/* Assets */}
          {showLayer.assets && assets.map(asset => (
            <CircleMarker
              key={asset.id}
              center={[asset.location.lat, asset.location.lng]}
              radius={10}
              fillColor={getAssetColor(asset)}
              color={getAssetColor(asset)}
              weight={2}
              opacity={0.9}
              fillOpacity={0.3}
            >
              <Popup>
                <div className="map-popup">
                  <h4>{asset.name}</h4>
                  <div className="map-popup-row">
                    <span>Category:</span><span>{asset.category}</span>
                  </div>
                  <div className="map-popup-row">
                    <span>Condition:</span>
                    <span style={{ color: conditionColor(asset.condition) }}>{asset.condition}</span>
                  </div>
                  <div className="map-popup-row">
                    <span>Risk Score:</span>
                    <span style={{ color: getAssetColor(asset) }}>{asset.riskScore}/100</span>
                  </div>
                  <div className="map-popup-row">
                    <span>Sensors:</span><span>{asset.sensors.length}</span>
                  </div>
                </div>
              </Popup>
            </CircleMarker>
          ))}

          {/* Earthquakes from USGS */}
          {showLayer.earthquakes && allEarthquakes.map(eq => (
            <CircleMarker
              key={eq.id}
              center={[eq.position.lat, eq.position.lng]}
              radius={getQuakeRadius(eq)}
              fillColor={severityColor(eq.severity || 'info').replace('var(--severity-', '').replace(')', '')}
              color="#ef4444"
              weight={1}
              opacity={0.7}
              fillOpacity={0.4}
            >
              <Popup>
                <div className="map-popup">
                  <h4>{eq.label}</h4>
                  <div className="map-popup-row">
                    <span>Magnitude:</span><span>{eq.details.magnitude as number}</span>
                  </div>
                  <div className="map-popup-row">
                    <span>Depth:</span><span>{(eq.details.depth_km as number)?.toFixed(1)} km</span>
                  </div>
                  {eq.details.time && (
                    <div className="map-popup-row">
                      <span>Time:</span><span>{new Date(eq.details.time as string).toLocaleString()}</span>
                    </div>
                  )}
                  <div className="map-popup-row">
                    <span>Source:</span><span>USGS Earthquake Hazards</span>
                  </div>
                </div>
              </Popup>
            </CircleMarker>
          ))}

          {/* Alert Locations */}
          {showLayer.alerts && criticalAlerts.filter(a => a.location).map(alert => (
            <CircleMarker
              key={alert.id}
              center={[alert.location!.lat, alert.location!.lng]}
              radius={14}
              fillColor={alert.severity === 'emergency' ? '#ef4444' : '#f97316'}
              color={alert.severity === 'emergency' ? '#ef4444' : '#f97316'}
              weight={3}
              opacity={0.9}
              fillOpacity={0.2}
            >
              <Popup>
                <div className="map-popup">
                  <h4>{alert.title}</h4>
                  <div className="map-popup-row">
                    <span>Severity:</span>
                    <span style={{ color: alert.severity === 'emergency' ? '#ef4444' : '#f97316' }}>
                      {alert.severity}
                    </span>
                  </div>
                  <div className="map-popup-row">
                    <span>Source:</span><span>{alert.source}</span>
                  </div>
                  <p style={{ fontSize: 11, color: '#94a3b8', marginTop: 6 }}>
                    {alert.description.slice(0, 150)}...
                  </p>
                </div>
              </Popup>
            </CircleMarker>
          ))}
        </MapContainer>

        {/* Map Legend */}
        <div className="map-legend">
          <h4>Legend</h4>
          <div className="legend-item">
            <span className="legend-dot" style={{ background: '#22c55e' }} />
            <span>Sensor (Online)</span>
          </div>
          <div className="legend-item">
            <span className="legend-dot" style={{ background: '#eab308' }} />
            <span>Sensor (Degraded)</span>
          </div>
          <div className="legend-item">
            <span className="legend-dot" style={{ background: '#ef4444' }} />
            <span>Sensor (Offline)</span>
          </div>
          <div className="legend-item">
            <span className="legend-ring" style={{ borderColor: '#22c55e' }} />
            <span>Asset (Low Risk)</span>
          </div>
          <div className="legend-item">
            <span className="legend-ring" style={{ borderColor: '#f97316' }} />
            <span>Asset (Med Risk)</span>
          </div>
          <div className="legend-item">
            <span className="legend-ring" style={{ borderColor: '#ef4444' }} />
            <span>Asset (High Risk)</span>
          </div>
          <div className="legend-item">
            <span className="legend-dot pulse" style={{ background: '#ef4444' }} />
            <span>USGS Earthquake</span>
          </div>
        </div>
      </div>
    </div>
  );
}
