import { useEffect, useRef, useState } from "react";
import { streamURL } from "../api/sunny";
import type { Record as SunnyRecord } from "../api/types";

export interface LiveStreamOptions {
  connector?: string;
  bufferSize?: number;
  replay?: boolean;
}

export interface LiveStreamState {
  records: SunnyRecord[];
  connected: boolean;
  recordsPerSec: number;
  total: number;
}

const DEFAULT_BUFFER = 200;

// useLiveStream subscribes to /api/stream over WebSocket and exposes the
// last N records (newest first), plus a 5-second-windowed records-per-second
// estimate. Reconnects with backoff on disconnect.
export function useLiveStream(opts: LiveStreamOptions = {}): LiveStreamState {
  const { connector, replay, bufferSize = DEFAULT_BUFFER } = opts;
  const [records, setRecords] = useState<SunnyRecord[]>([]);
  const [connected, setConnected] = useState(false);
  const [recordsPerSec, setRecordsPerSec] = useState(0);
  const [total, setTotal] = useState(0);

  const buf = useRef<SunnyRecord[]>([]);
  const stamps = useRef<number[]>([]);
  const totalRef = useRef(0);

  useEffect(() => {
    let ws: WebSocket | null = null;
    let stopped = false;
    let backoff = 500;
    let rafId: number | null = null;

    const flush = () => {
      rafId = null;
      // Trim buffer to bufferSize and update state.
      if (buf.current.length > bufferSize) {
        buf.current = buf.current.slice(0, bufferSize);
      }
      setRecords([...buf.current]);
    };

    const scheduleFlush = () => {
      if (rafId !== null) return;
      rafId = requestAnimationFrame(flush);
    };

    const computeRate = () => {
      const now = Date.now();
      const cutoff = now - 5000;
      stamps.current = stamps.current.filter((s) => s >= cutoff);
      setRecordsPerSec(stamps.current.length / 5);
    };
    const rateTimer = window.setInterval(computeRate, 1000);

    const connect = () => {
      if (stopped) return;
      const url = streamURL({ connector, replay });
      ws = new WebSocket(url);

      ws.onopen = () => {
        setConnected(true);
        backoff = 500;
      };
      ws.onmessage = (ev) => {
        try {
          const r = JSON.parse(ev.data as string) as SunnyRecord;
          buf.current = [r, ...buf.current];
          stamps.current.push(Date.now());
          totalRef.current++;
          setTotal(totalRef.current);
          scheduleFlush();
        } catch {
          // ignore malformed frames
        }
      };
      ws.onclose = () => {
        setConnected(false);
        if (stopped) return;
        const delay = Math.min(backoff, 8000);
        backoff *= 2;
        window.setTimeout(connect, delay);
      };
      ws.onerror = () => {
        // close handler will retry
        ws?.close();
      };
    };

    connect();

    return () => {
      stopped = true;
      ws?.close();
      window.clearInterval(rateTimer);
      if (rafId !== null) cancelAnimationFrame(rafId);
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [connector, replay, bufferSize]);

  return { records, connected, recordsPerSec, total };
}
