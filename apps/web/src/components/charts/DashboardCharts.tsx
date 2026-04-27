// Recharts is heavy (~250 KB). Keep all the chart components in one file
// behind a single dynamic import in Dashboard.tsx so the rest of the
// dashboard can render before the chart code arrives.
import {
  Area,
  AreaChart,
  Bar,
  BarChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";

export interface ThroughputDatum {
  time: string;
  count: number;
}

export function ThroughputChart({ data }: { data: ThroughputDatum[] }) {
  return (
    <ResponsiveContainer width="100%" height={200}>
      <AreaChart data={data}>
        <defs>
          <linearGradient id="ingestGrad" x1="0" y1="0" x2="0" y2="1">
            <stop offset="5%" stopColor="var(--kafka-accent)" stopOpacity={0.3} />
            <stop offset="95%" stopColor="var(--kafka-accent)" stopOpacity={0} />
          </linearGradient>
        </defs>
        <CartesianGrid strokeDasharray="3 3" stroke="var(--border-primary)" />
        <XAxis
          dataKey="time"
          tick={{ fill: "var(--text-tertiary)", fontSize: 10 }}
          tickLine={false}
          axisLine={false}
        />
        <YAxis
          tick={{ fill: "var(--text-tertiary)", fontSize: 10 }}
          tickLine={false}
          axisLine={false}
        />
        <Tooltip
          contentStyle={{
            background: "var(--bg-elevated)",
            border: "1px solid var(--border-secondary)",
            borderRadius: 8,
            fontSize: 12,
          }}
        />
        <Area
          type="monotone"
          dataKey="count"
          stroke="var(--kafka-accent)"
          fill="url(#ingestGrad)"
          strokeWidth={2}
        />
      </AreaChart>
    </ResponsiveContainer>
  );
}

export interface SeverityDatum {
  name: string;
  count: number;
  fill: string;
}

export function SeverityBarChart({ data }: { data: SeverityDatum[] }) {
  return (
    <ResponsiveContainer width="100%" height={200}>
      <BarChart data={data} layout="vertical">
        <CartesianGrid strokeDasharray="3 3" stroke="var(--border-primary)" />
        <XAxis
          type="number"
          tick={{ fill: "var(--text-tertiary)", fontSize: 10 }}
          tickLine={false}
          axisLine={false}
        />
        <YAxis
          dataKey="name"
          type="category"
          tick={{ fill: "var(--text-tertiary)", fontSize: 11 }}
          tickLine={false}
          axisLine={false}
          width={80}
        />
        <Tooltip
          contentStyle={{
            background: "var(--bg-elevated)",
            border: "1px solid var(--border-secondary)",
            borderRadius: 8,
            fontSize: 12,
          }}
        />
        <Bar dataKey="count" radius={[0, 4, 4, 0]} barSize={20} />
      </BarChart>
    </ResponsiveContainer>
  );
}
