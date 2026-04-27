import { describe, it, expect, beforeEach, afterEach } from "vitest";
import { renderHook, act, waitFor } from "@testing-library/react";
import { useLiveStream } from "./useLiveStream";

// Minimal WebSocket stub. Tracks instances so the test can drive open/message/close.
class FakeWS {
  static instances: FakeWS[] = [];
  url: string;
  onopen: ((e: Event) => void) | null = null;
  onmessage: ((e: MessageEvent) => void) | null = null;
  onclose: ((e: CloseEvent) => void) | null = null;
  onerror: ((e: Event) => void) | null = null;
  closed = false;
  constructor(url: string) {
    this.url = url;
    FakeWS.instances.push(this);
    queueMicrotask(() => this.onopen?.(new Event("open")));
  }
  send() {}
  close() {
    if (this.closed) return;
    this.closed = true;
    queueMicrotask(() => this.onclose?.(new CloseEvent("close")));
  }
  emit(data: unknown) {
    this.onmessage?.(new MessageEvent("message", { data: JSON.stringify(data) }));
  }
}

const realWS = globalThis.WebSocket;
const realRAF = globalThis.requestAnimationFrame;

beforeEach(() => {
  FakeWS.instances = [];
  // @ts-expect-error overriding for tests
  globalThis.WebSocket = FakeWS;
  // jsdom may lack RAF; force it onto a microtask flush.
  globalThis.requestAnimationFrame = ((cb: FrameRequestCallback) => {
    queueMicrotask(() => cb(performance.now()));
    return 0;
  }) as typeof requestAnimationFrame;
  Object.defineProperty(window, "location", {
    writable: true,
    value: new URL("http://localhost:3000/"),
  });
});

afterEach(() => {
  globalThis.WebSocket = realWS;
  globalThis.requestAnimationFrame = realRAF;
});

describe("useLiveStream", () => {
  it("connects and exposes connected=true", async () => {
    const { result } = renderHook(() => useLiveStream({ bufferSize: 50 }));
    await waitFor(() => expect(result.current.connected).toBe(true));
  });

  it("appends incoming records to state", async () => {
    const { result } = renderHook(() => useLiveStream({ bufferSize: 50 }));
    await waitFor(() => expect(result.current.connected).toBe(true));

    const ws = FakeWS.instances[0];
    act(() => {
      ws.emit({ timestamp: "2026-01-01T00:00:00Z", connectorId: "c1", payload: {} });
      ws.emit({ timestamp: "2026-01-01T00:00:01Z", connectorId: "c1", payload: {} });
    });

    await waitFor(() => expect(result.current.records.length).toBe(2));
    // Newest first.
    expect(result.current.records[0].timestamp).toBe("2026-01-01T00:00:01Z");
  });

  it("respects bufferSize cap", async () => {
    const { result } = renderHook(() => useLiveStream({ bufferSize: 3 }));
    await waitFor(() => expect(result.current.connected).toBe(true));

    const ws = FakeWS.instances[0];
    act(() => {
      for (let i = 0; i < 10; i++) {
        ws.emit({
          timestamp: `2026-01-01T00:00:0${i}Z`,
          connectorId: "c",
          payload: {},
        });
      }
    });

    await waitFor(() => expect(result.current.records.length).toBe(3));
    // total counts every received message regardless of cap
    expect(result.current.total).toBe(10);
  });

  it("reconnects on close", async () => {
    const { result } = renderHook(() => useLiveStream({ bufferSize: 10 }));
    await waitFor(() => expect(result.current.connected).toBe(true));

    const first = FakeWS.instances[0];
    act(() => first.close());
    await waitFor(() => expect(result.current.connected).toBe(false));

    // Backoff schedules a new connect; advance microtasks until a 2nd ws appears.
    await waitFor(() => expect(FakeWS.instances.length).toBeGreaterThan(1), {
      timeout: 1500,
    });
  });

  it("includes connector filter in URL", async () => {
    renderHook(() => useLiveStream({ connector: "earthquakes" }));
    await waitFor(() => expect(FakeWS.instances.length).toBe(1));
    expect(FakeWS.instances[0].url).toContain("connector=earthquakes");
  });
});
