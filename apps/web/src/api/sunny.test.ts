import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import {
  getConnectors,
  getRecords,
  getAuthStatus,
  login,
  ackAlert,
  streamURL,
} from "./sunny";

const realFetch = globalThis.fetch;

beforeEach(() => {
  globalThis.fetch = vi.fn();
});

afterEach(() => {
  globalThis.fetch = realFetch;
});

function mockJSON(body: unknown, status = 200) {
  (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
    ok: status >= 200 && status < 300,
    status,
    statusText: status === 200 ? "OK" : "Error",
    json: () => Promise.resolve(body),
  });
}

describe("sunny api client", () => {
  it("getConnectors hits /api/connectors with credentials", async () => {
    mockJSON({ types: [], instances: [] });
    const r = await getConnectors();
    expect(r).toEqual({ types: [], instances: [] });
    const call = (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toContain("/api/connectors");
    expect(call[1]).toMatchObject({ credentials: "include" });
  });

  it("getRecords serializes query params", async () => {
    mockJSON([]);
    await getRecords({ connector: "c1", from: "2026-01-01T00:00:00Z", limit: 50 });
    const url = (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("connector=c1");
    expect(url).toContain("from=2026-01-01");
    expect(url).toContain("limit=50");
  });

  it("getRecords coerces null response to []", async () => {
    mockJSON(null);
    const r = await getRecords();
    expect(r).toEqual([]);
  });

  it("getAuthStatus returns enabled flag", async () => {
    mockJSON({ enabled: true, loggedIn: false });
    const s = await getAuthStatus();
    expect(s.enabled).toBe(true);
    expect(s.loggedIn).toBe(false);
  });

  it("login throws on 401", async () => {
    (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: false,
      status: 401,
    });
    await expect(login("wrong")).rejects.toThrow();
  });

  it("login resolves on 200", async () => {
    (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      status: 200,
    });
    await expect(login("good")).resolves.toBeUndefined();
  });

  it("login resolves on 204 (auth disabled)", async () => {
    (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      status: 204,
    });
    await expect(login("anything")).resolves.toBeUndefined();
  });

  it("ackAlert sends POST", async () => {
    (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mockResolvedValueOnce({
      ok: true,
      status: 200,
    });
    await ackAlert("alert-123");
    const call = (globalThis.fetch as unknown as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toContain("/api/alerts/alert-123/ack");
    expect(call[1]).toMatchObject({ method: "POST", credentials: "include" });
  });
});

describe("streamURL", () => {
  beforeEach(() => {
    Object.defineProperty(window, "location", {
      writable: true,
      value: new URL("http://localhost:3000/"),
    });
  });

  it("builds ws:// URL same-origin", () => {
    const u = streamURL();
    expect(u).toMatch(/^ws:\/\/localhost:3000\/api\/stream/);
  });

  it("includes connector filter", () => {
    const u = streamURL({ connector: "earthquakes" });
    expect(u).toContain("connector=earthquakes");
  });

  it("includes replay flag", () => {
    const u = streamURL({ replay: true });
    expect(u).toContain("replay=1");
  });
});
