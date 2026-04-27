// Mirror of the Go SDK's wire types. Keep in sync with packages/sdk-go/connector.go
// and apps/server/internal/connectors/runtime.go.

export type Mode = "pull" | "push" | "stream";

export type Category =
  | "geophysical"
  | "weather"
  | "air_quality"
  | "hydrology"
  | "wildfire"
  | "structural"
  | "iot"
  | "industrial"
  | "custom";

export interface Manifest {
  id: string;
  name: string;
  version: string;
  category: Category;
  mode: Mode;
  description: string;
  configSchema: unknown; // JSON Schema; rendered by the config form generator
}

export type InstanceState = "starting" | "running" | "backoff" | "stopped" | "failed";

export interface InstanceStatus {
  instanceId: string;
  type: string;
  state: InstanceState;
  startedAt: string;
  restarts: number;
  lastError?: string;
  lastErrorAt?: string;
}

export interface ConnectorsResponse {
  types: Manifest[];
  instances: InstanceStatus[];
}

export interface GeoPoint {
  lat: number;
  lng: number;
  altitude?: number;
}

export interface Record {
  timestamp: string;
  connectorId: string;
  sourceId?: string;
  location?: GeoPoint;
  tags?: globalThis.Record<string, string>;
  payload: unknown;
}

export interface RecordsQuery {
  connector?: string;
  from?: string; // RFC3339
  to?: string; // RFC3339
  limit?: number;
}

export interface TimeseriesBucket {
  bucket: string; // RFC3339
  count: number;
}

export interface AlertRule {
  id: string;
  name: string;
  enabled: boolean;
  connectorId?: string;
  severityIn?: string[];
  tagEquals?: globalThis.Record<string, string>;
  createdAt: string;
}

export interface Alert {
  id: string;
  ruleId: string;
  ruleName: string;
  connectorId: string;
  sourceId?: string;
  severity: string;
  headline: string;
  tags?: globalThis.Record<string, string>;
  payload?: unknown;
  triggered: string;
  acked?: string;
}

// --- Registry (bundled docs/registry.json) ---

export interface RegistrySecret {
  name: string;
  description?: string;
  envExample?: string;
  obtainUrl?: string;
}

export interface RegistryEntry {
  id: string;
  name: string;
  description?: string;
  category: Category;
  mode: Mode;
  version?: string;
  source: { type: string; module?: string; endpoint?: string };
  secrets?: RegistrySecret[];
  configSchema?: unknown;
  homepage?: string;
  license?: string;
  maintainers?: string[];
  verified?: boolean;
}

export interface RegistryDocument {
  version: string;
  updated?: string;
  connectors: RegistryEntry[];
}

