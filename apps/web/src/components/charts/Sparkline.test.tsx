import { describe, it, expect } from "vitest";
import { render } from "@testing-library/react";
import { Sparkline } from "./Sparkline";

describe("Sparkline", () => {
  it("renders dashed placeholder for <2 values", () => {
    const { container } = render(<Sparkline values={[]} />);
    const line = container.querySelector("line");
    expect(line).not.toBeNull();
    expect(line?.getAttribute("stroke-dasharray")).toBeTruthy();
  });

  it("renders polyline for ≥2 values", () => {
    const { container } = render(<Sparkline values={[1, 2, 3, 2, 5]} width={100} height={20} />);
    const poly = container.querySelector("polyline");
    expect(poly).not.toBeNull();
    const points = poly?.getAttribute("points") ?? "";
    expect(points.split(" ").length).toBe(5);
  });

  it("scales to viewBox", () => {
    const { container } = render(<Sparkline values={[1, 5, 10]} width={120} height={30} />);
    const svg = container.querySelector("svg");
    expect(svg?.getAttribute("viewBox")).toBe("0 0 120 30");
  });

  it("draws fill path when fill is provided", () => {
    const { container } = render(
      <Sparkline values={[1, 2, 3]} fill="rgba(0,0,0,0.2)" />,
    );
    const paths = container.querySelectorAll("path");
    expect(paths.length).toBeGreaterThanOrEqual(1);
  });
});
