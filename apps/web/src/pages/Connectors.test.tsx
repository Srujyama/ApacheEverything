import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import Connectors from "./Connectors";
import * as api from "../api/sunny";
import type { ConnectorsResponse, RegistryDocument } from "../api/types";

const sampleConnectors: ConnectorsResponse = {
  types: [
    {
      id: "usgs-earthquakes",
      name: "USGS Earthquakes",
      version: "0.1.0",
      category: "geophysical",
      mode: "pull",
      description: "Earthquakes feed.",
      configSchema: { type: "object", properties: {} },
    },
    {
      id: "webhook",
      name: "Generic Webhook",
      version: "0.1.0",
      category: "custom",
      mode: "push",
      description: "Inbound HTTP push receiver.",
      configSchema: { type: "object", properties: {} },
    },
  ],
  instances: [
    {
      instanceId: "earthquakes",
      type: "usgs-earthquakes",
      state: "running",
      startedAt: new Date().toISOString(),
      restarts: 0,
    },
    {
      instanceId: "my-webhook",
      type: "webhook",
      state: "running",
      startedAt: new Date().toISOString(),
      restarts: 2,
    },
  ],
};

const sampleRegistry: RegistryDocument = {
  version: "1",
  connectors: [
    {
      id: "usgs-earthquakes",
      name: "USGS Earthquakes",
      mode: "pull",
      category: "geophysical",
      version: "0.1.0",
      source: { type: "builtin" },
      verified: true,
      homepage: "https://earthquake.usgs.gov/",
    },
    {
      id: "webhook",
      name: "Generic Webhook",
      mode: "push",
      category: "custom",
      version: "0.1.0",
      source: { type: "builtin" },
      verified: true,
    },
  ],
};

beforeEach(() => {
  vi.spyOn(api, "getConnectors").mockResolvedValue(sampleConnectors);
  vi.spyOn(api, "getConnectorRegistry").mockResolvedValue(sampleRegistry);
  // Stub timeseries so InstanceCard's sparkline fetch doesn't reject in tests.
  vi.spyOn(api, "getTimeseries").mockResolvedValue([]);
});

afterEach(() => {
  vi.restoreAllMocks();
});

describe("Connectors page", () => {
  it("renders both registered types as marketplace tiles", async () => {
    render(<Connectors />);
    await waitFor(() => {
      expect(screen.getByText("USGS Earthquakes")).toBeInTheDocument();
      expect(screen.getByText("Generic Webhook")).toBeInTheDocument();
    });
  });

  it("groups tiles by category with section headings", async () => {
    render(<Connectors />);
    await waitFor(() => {
      expect(screen.getByText("Geophysical")).toBeInTheDocument();
      expect(screen.getByText("Custom")).toBeInTheDocument();
    });
  });

  it("shows running instances as cards", async () => {
    render(<Connectors />);
    await waitFor(() => {
      expect(screen.getByText("earthquakes")).toBeInTheDocument();
      expect(screen.getByText("my-webhook")).toBeInTheDocument();
    });
  });

  it("renders push-mode ingest URL on the webhook instance card", async () => {
    render(<Connectors />);
    await waitFor(() => {
      expect(screen.getByText("/api/ingest/my-webhook/")).toBeInTheDocument();
    });
  });

  it("shows restart-count warning on restarted instances", async () => {
    render(<Connectors />);
    await waitFor(() => {
      expect(screen.getByText(/restarts: 2/)).toBeInTheDocument();
    });
  });

  it("opens the manifest modal when a tile is clicked", async () => {
    render(<Connectors />);
    await waitFor(() => screen.getByText("USGS Earthquakes"));
    fireEvent.click(screen.getByText("USGS Earthquakes"));
    await waitFor(() => {
      // Modal renders the install snippet referencing the type id.
      expect(screen.getByText(/my-usgs-earthquakes/)).toBeInTheDocument();
    });
  });

  it("renders curl example for push connectors in the modal", async () => {
    render(<Connectors />);
    await waitFor(() => screen.getByText("Generic Webhook"));
    fireEvent.click(screen.getByText("Generic Webhook"));
    await waitFor(() => {
      // Push-mode modal contains a curl snippet with the ingest path.
      const codeBlocks = screen.getAllByText(
        /curl -X POST .*\/api\/ingest\/my-webhook\//,
      );
      expect(codeBlocks.length).toBeGreaterThan(0);
    });
  });
});
