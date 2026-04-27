import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import AuthGate from "./AuthGate";
import * as api from "../api/sunny";

const realFetch = globalThis.fetch;

beforeEach(() => {
  vi.spyOn(api, "getAuthStatus").mockReset();
  vi.spyOn(api, "login").mockReset();
});

afterEach(() => {
  globalThis.fetch = realFetch;
  vi.restoreAllMocks();
});

describe("AuthGate", () => {
  it("renders children when auth is disabled", async () => {
    vi.spyOn(api, "getAuthStatus").mockResolvedValue({ enabled: false, loggedIn: true });
    render(
      <AuthGate>
        <div>app content</div>
      </AuthGate>,
    );
    await waitFor(() => expect(screen.getByText("app content")).toBeInTheDocument());
  });

  it("renders children when already logged in", async () => {
    vi.spyOn(api, "getAuthStatus").mockResolvedValue({ enabled: true, loggedIn: true });
    render(
      <AuthGate>
        <div>app content</div>
      </AuthGate>,
    );
    await waitFor(() => expect(screen.getByText("app content")).toBeInTheDocument());
  });

  it("renders login form when auth required and not logged in", async () => {
    vi.spyOn(api, "getAuthStatus").mockResolvedValue({ enabled: true, loggedIn: false });
    render(
      <AuthGate>
        <div>app content</div>
      </AuthGate>,
    );
    await waitFor(() => expect(screen.getByText(/Sign in/)).toBeInTheDocument());
    expect(screen.queryByText("app content")).not.toBeInTheDocument();
  });

  it("renders login form on getAuthStatus error", async () => {
    vi.spyOn(api, "getAuthStatus").mockRejectedValue(new Error("network"));
    render(
      <AuthGate>
        <div>app content</div>
      </AuthGate>,
    );
    await waitFor(() => expect(screen.getByText(/Sign in/)).toBeInTheDocument());
  });
});
