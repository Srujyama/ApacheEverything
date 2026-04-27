import type {
  Alert,
  AlertRule,
  ConnectorsResponse,
  InstanceStatus,
  Record as SunnyRecord,
  RecordsQuery,
  RegistryDocument,
  TimeseriesBucket,
} from "./types";

// Same-origin in production (Go server serves the SPA at the same port).
// In dev, vite.config.ts proxies /api over to the backend, so we can also
// use relative URLs there. VITE_API_BASE overrides for split deployments.
const API_BASE =
  (import.meta as { env?: { VITE_API_BASE?: string } }).env?.VITE_API_BASE ?? "";

async function getJSON<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    credentials: "include",
    ...init,
  });
  if (!res.ok) {
    throw new Error(`${path}: ${res.status} ${res.statusText}`);
  }
  return res.json() as Promise<T>;
}

export interface AuthStatus {
  enabled: boolean;
  loggedIn: boolean;
}

export function getAuthStatus(): Promise<AuthStatus> {
  return getJSON<AuthStatus>("/api/auth/status");
}

export async function login(password: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/auth/login`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ password }),
  });
  if (res.status === 204) return; // auth disabled
  if (!res.ok) throw new Error("Invalid password");
}

export async function logout(): Promise<void> {
  await fetch(`${API_BASE}/api/auth/logout`, {
    method: "POST",
    credentials: "include",
  });
}

export function getConnectors(): Promise<ConnectorsResponse> {
  return getJSON<ConnectorsResponse>("/api/connectors");
}

// Lighter than getConnectors() — returns just the running-instance array.
// Use this for high-frequency polling (e.g. the sidebar).
export function getConnectorInstances(): Promise<InstanceStatus[]> {
  return getJSON<InstanceStatus[] | null>("/api/connectors/instances").then(
    (v) => v ?? [],
  );
}

export function getConnectorRegistry(): Promise<RegistryDocument> {
  return getJSON<RegistryDocument>("/api/connectors/registry");
}

export function getInstance(id: string): Promise<InstanceStatus> {
  return getJSON<InstanceStatus>(`/api/connectors/${encodeURIComponent(id)}`);
}

export function getRecords(q: RecordsQuery = {}): Promise<SunnyRecord[]> {
  const params = new URLSearchParams();
  if (q.connector) params.set("connector", q.connector);
  if (q.from) params.set("from", q.from);
  if (q.to) params.set("to", q.to);
  if (q.limit !== undefined) params.set("limit", String(q.limit));
  const qs = params.toString();
  return getJSON<SunnyRecord[]>(`/api/records${qs ? `?${qs}` : ""}`).then(
    (v) => v ?? [],
  );
}

export function getHealth(): Promise<{ status: string; time: string }> {
  return getJSON("/api/health");
}

export function getRecordCounts(): Promise<globalThis.Record<string, number>> {
  return getJSON("/api/records/counts");
}

export interface TimeseriesQuery {
  connector?: string;
  from?: string;
  to?: string;
  bucketSeconds?: number;
}

export function getTimeseries(q: TimeseriesQuery = {}): Promise<TimeseriesBucket[]> {
  const params = new URLSearchParams();
  if (q.connector) params.set("connector", q.connector);
  if (q.from) params.set("from", q.from);
  if (q.to) params.set("to", q.to);
  if (q.bucketSeconds !== undefined) params.set("bucket", String(q.bucketSeconds));
  const qs = params.toString();
  return getJSON<TimeseriesBucket[]>(`/api/timeseries${qs ? `?${qs}` : ""}`).then(
    (v) => v ?? [],
  );
}

export function getAlerts(limit = 100): Promise<Alert[]> {
  return getJSON<Alert[]>(`/api/alerts?limit=${limit}`).then((v) => v ?? []);
}

export async function ackAlert(id: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/alerts/${encodeURIComponent(id)}/ack`, {
    method: "POST",
    credentials: "include",
  });
  if (!res.ok) throw new Error(`ack: ${res.status}`);
}

export function getAlertRules(): Promise<AlertRule[]> {
  return getJSON<AlertRule[]>("/api/alerts/rules").then((v) => v ?? []);
}

export async function saveAlertRule(rule: Partial<AlertRule>): Promise<AlertRule> {
  const res = await fetch(`${API_BASE}/api/alerts/rules`, {
    method: "POST",
    credentials: "include",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(rule),
  });
  if (!res.ok) throw new Error(`save rule: ${res.status}`);
  return res.json() as Promise<AlertRule>;
}

export async function deleteAlertRule(id: string): Promise<void> {
  const res = await fetch(`${API_BASE}/api/alerts/rules/${encodeURIComponent(id)}`, {
    method: "DELETE",
    credentials: "include",
  });
  if (!res.ok) throw new Error(`delete rule: ${res.status}`);
}

// streamURL returns the WebSocket URL for /api/stream, optionally filtered.
export function streamURL(opts: { connector?: string; replay?: boolean } = {}): string {
  // Build absolute ws:// URL relative to the API base if same-origin, or
  // explicit host in dev mode.
  const httpBase = API_BASE || window.location.origin;
  const url = new URL("/api/stream", httpBase);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  if (opts.connector) url.searchParams.set("connector", opts.connector);
  if (opts.replay) url.searchParams.set("replay", "1");
  return url.toString();
}
