/**
 * @sunny/sdk-ts — Sunny connector SDK for TypeScript.
 *
 * Public surface (frozen at v1):
 *   - SDK_VERSION constant
 *   - Mode, Category enums (mirror Go SDK Mode, Category)
 *   - Manifest, Record, GeoPoint, Logger interfaces
 *   - SunnyContext (mirror of Go's sdk.Context)
 *   - Connector interface
 *
 * Anything added here MUST mirror the Go SDK in packages/sdk-go. Connectors
 * authored in TypeScript will talk to the Go runtime over the JSON-RPC
 * bridge that ships with Phase 7 (streaming + out-of-process connectors);
 * v1 freezes the schema now so that contract can ship without surprises.
 */

export const SDK_VERSION = "1.0.0";

export type Mode = "pull" | "push" | "stream";

export const ModePull: Mode = "pull";
export const ModePush: Mode = "push";
export const ModeStream: Mode = "stream";

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
  /** JSON Schema (draft 2020-12) for this connector's config. */
  configSchema: unknown;
}

export interface GeoPoint {
  lat: number;
  lng: number;
  altitude?: number;
}

export interface Record {
  /** ISO-8601 with timezone. */
  timestamp: string;
  connectorId: string;
  sourceId?: string;
  location?: GeoPoint;
  tags?: { [k: string]: string };
  /** Arbitrary connector payload. */
  payload: unknown;
}

export interface Logger {
  debug(msg: string, ...attrs: unknown[]): void;
  info(msg: string, ...attrs: unknown[]): void;
  warn(msg: string, ...attrs: unknown[]): void;
  error(msg: string, ...attrs: unknown[]): void;
}

/** SunnyContext is the runtime surface a connector sees. */
export interface SunnyContext {
  publish(record: Record): Promise<void>;
  logger(): Logger;
  /** Returns "" when the secret is unset. */
  secret(name: string): string;
  checkpoint(key: string, value: string): Promise<void>;
  loadCheckpoint(key: string): Promise<string>;
}

/** Pull connectors implement run(). Push/stream get distinct entry points
 * in v1.1 once the JSON-RPC bridge specifies them — keeping the interface
 * minimal here so future shapes are additive. */
export interface Connector {
  manifest(): Manifest;
  validate(config: unknown): void; // throws on invalid config
  run(ctx: SunnyContext, config: unknown, signal: AbortSignal): Promise<void>;
}
