import { describe, it, expect } from "vitest";
import {
  formatBytes,
  formatNumber,
  formatDuration,
  formatTimeAgo,
  formatPercentage,
  severityColor,
  statusColor,
} from "./format";

describe("formatBytes", () => {
  it("formats zero", () => expect(formatBytes(0)).toBe("0 B"));
  it("formats KB", () => expect(formatBytes(2048)).toBe("2 KB"));
  it("formats MB", () => expect(formatBytes(5 * 1024 * 1024)).toBe("5 MB"));
  it("formats GB with decimals", () =>
    expect(formatBytes(1.5 * 1024 ** 3)).toBe("1.5 GB"));
});

describe("formatNumber", () => {
  it("renders small numbers verbatim", () => expect(formatNumber(42)).toBe("42"));
  it("renders thousands with K", () => expect(formatNumber(2500)).toBe("2.5K"));
  it("renders millions with M", () => expect(formatNumber(3_400_000)).toBe("3.4M"));
});

describe("formatDuration", () => {
  it("ms", () => expect(formatDuration(250)).toBe("250ms"));
  it("seconds", () => expect(formatDuration(1500)).toBe("1.5s"));
  it("minutes", () => expect(formatDuration(150_000)).toBe("2m 30s"));
  it("hours", () => expect(formatDuration(3_900_000)).toBe("1h 5m"));
});

describe("formatTimeAgo", () => {
  it("'Just now' for sub-minute", () => {
    const t = new Date(Date.now() - 30_000).toISOString();
    expect(formatTimeAgo(t)).toBe("Just now");
  });
  it("minutes", () => {
    const t = new Date(Date.now() - 5 * 60_000).toISOString();
    expect(formatTimeAgo(t)).toBe("5m ago");
  });
  it("hours", () => {
    const t = new Date(Date.now() - 2 * 3_600_000).toISOString();
    expect(formatTimeAgo(t)).toBe("2h ago");
  });
});

describe("formatPercentage", () => {
  it("zero total → 0%", () => expect(formatPercentage(5, 0)).toBe("0%"));
  it("ratio", () => expect(formatPercentage(25, 100)).toBe("25.0%"));
});

describe("severityColor / statusColor", () => {
  it("severity maps to CSS variables", () => {
    expect(severityColor("emergency")).toContain("--severity-emergency");
    expect(severityColor("info")).toContain("--severity-info");
  });
  it("status running is good", () => {
    expect(statusColor("running")).toContain("--status-good");
  });
  it("unknown status falls back", () => {
    expect(statusColor("anything")).toContain("--text-secondary");
  });
});
