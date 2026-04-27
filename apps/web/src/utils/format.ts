// ============================================================================
// Formatting Utilities
// ============================================================================

export function formatBytes(bytes: number): string {
  if (bytes === 0) return '0 B';
  const k = 1024;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
}

export function formatNumber(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}K`;
  return n.toLocaleString();
}

export function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)}s`;
  if (ms < 3600000) return `${Math.floor(ms / 60000)}m ${Math.floor((ms % 60000) / 1000)}s`;
  return `${Math.floor(ms / 3600000)}h ${Math.floor((ms % 3600000) / 60000)}m`;
}

export function formatTimestamp(iso: string): string {
  const d = new Date(iso);
  return d.toLocaleString('en-US', {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
}

export function formatTimeAgo(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime();
  if (diff < 60000) return 'Just now';
  if (diff < 3600000) return `${Math.floor(diff / 60000)}m ago`;
  if (diff < 86400000) return `${Math.floor(diff / 3600000)}h ago`;
  return `${Math.floor(diff / 86400000)}d ago`;
}

export function formatPercentage(value: number, total: number): string {
  if (total === 0) return '0%';
  return `${((value / total) * 100).toFixed(1)}%`;
}

export function severityColor(severity: string): string {
  switch (severity) {
    case 'emergency': return 'var(--severity-emergency)';
    case 'critical': return 'var(--severity-critical)';
    case 'warning': return 'var(--severity-warning)';
    case 'info': return 'var(--severity-info)';
    default: return 'var(--text-secondary)';
  }
}

export function conditionColor(condition: string): string {
  switch (condition) {
    case 'optimal': return 'var(--status-optimal)';
    case 'good': return 'var(--status-good)';
    case 'fair': return 'var(--status-fair)';
    case 'degraded': return 'var(--severity-warning)';
    case 'critical': return 'var(--severity-critical)';
    default: return 'var(--text-secondary)';
  }
}

export function statusColor(status: string): string {
  switch (status) {
    case 'online':
    case 'active':
    case 'connected':
    case 'running':
      return 'var(--status-good)';
    case 'degraded':
    case 'rate_limited':
      return 'var(--severity-warning)';
    case 'offline':
    case 'disconnected':
    case 'error':
    case 'failed':
      return 'var(--severity-critical)';
    case 'maintenance':
    case 'pending':
      return 'var(--text-secondary)';
    case 'completed':
      return 'var(--accent)';
    default:
      return 'var(--text-secondary)';
  }
}
