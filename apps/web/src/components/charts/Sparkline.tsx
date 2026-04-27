// Inline SVG sparkline. Deliberately *not* Recharts so the marketplace UI
// doesn't pull in 250 KB just to draw a 60-pixel line per tile.
//
// Renders a single polyline path scaled into the given width × height.

interface Props {
  values: number[];
  width?: number;
  height?: number;
  color?: string;
  strokeWidth?: number;
  fill?: string;
}

export function Sparkline({
  values,
  width = 80,
  height = 24,
  color = "currentColor",
  strokeWidth = 1.5,
  fill = "none",
}: Props) {
  if (values.length < 2) {
    return (
      <svg width={width} height={height} aria-label="sparkline (not enough data)">
        <line
          x1={0}
          y1={height / 2}
          x2={width}
          y2={height / 2}
          stroke={color}
          strokeOpacity={0.25}
          strokeWidth={strokeWidth}
          strokeDasharray="2 2"
        />
      </svg>
    );
  }

  const min = Math.min(...values);
  const max = Math.max(...values);
  const range = max - min || 1;

  const stepX = width / (values.length - 1);
  const points = values
    .map((v, i) => {
      const x = i * stepX;
      // Invert Y: SVG origin is top-left.
      const y = height - ((v - min) / range) * height;
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    })
    .join(" ");

  // Closed path for fill.
  const fillPath =
    fill !== "none"
      ? `M0,${height} L${points.replace(/ /g, " L")} L${width},${height} Z`
      : "";

  return (
    <svg width={width} height={height} viewBox={`0 0 ${width} ${height}`} aria-label="sparkline">
      {fill !== "none" && <path d={fillPath} fill={fill} />}
      <polyline
        points={points}
        fill="none"
        stroke={color}
        strokeWidth={strokeWidth}
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  );
}
