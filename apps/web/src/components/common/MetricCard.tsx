import type { ReactNode } from 'react';
import './MetricCard.css';

interface MetricCardProps {
  label: string;
  value: string | number;
  subtitle?: string;
  icon?: ReactNode;
  trend?: 'up' | 'down' | 'stable';
  trendValue?: string;
  accentColor?: string;
}

export default function MetricCard({ label, value, subtitle, icon, trend, trendValue, accentColor }: MetricCardProps) {
  return (
    <div className="metric-card card" style={accentColor ? { borderTopColor: accentColor } : undefined}>
      <div className="metric-card-header">
        <span className="metric-card-label">{label}</span>
        {icon && <span className="metric-card-icon">{icon}</span>}
      </div>
      <div className="metric-card-value">{value}</div>
      <div className="metric-card-footer">
        {subtitle && <span className="metric-card-subtitle">{subtitle}</span>}
        {trend && trendValue && (
          <span className={`metric-card-trend metric-trend-${trend}`}>
            {trend === 'up' ? '+' : trend === 'down' ? '-' : ''}{trendValue}
          </span>
        )}
      </div>
    </div>
  );
}
