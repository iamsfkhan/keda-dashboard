import { render, screen, waitFor } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";
import App from "./App";

class MockEventSource {
  addEventListener() {}
  close() {}
}

vi.stubGlobal("EventSource", MockEventSource);

afterEach(() => vi.restoreAllMocks());

describe("dashboard", () => {
  it("renders overview data", async () => {
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL) => {
      const url = input.toString();
      const body = url.includes("/meta")
        ? { kedaInstalled: true, kedaVersion: "v1alpha1", prometheusEnabled: false, cluster: "test" }
        : { scaledObjects: 3, scaledJobs: 1, ready: 3, active: 2, paused: 0, unhealthy: 1, triggerTypes: { kafka: 2 }, namespaces: ["default"] };
      return Promise.resolve(new Response(JSON.stringify(body), { status: 200, headers: { "Content-Type": "application/json" } }));
    }));
    render(<MemoryRouter><App /></MemoryRouter>);
    expect(await screen.findByText("Autoscaling, at a glance.")).toBeInTheDocument();
    await waitFor(() => expect(screen.getAllByText("3")).toHaveLength(2));
    expect(screen.getByText("kafka")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: /Active/ })).toHaveAttribute("href", "/scaledobjects?status=active");
    expect(screen.getByRole("link", { name: /Needs attention/ })).toHaveAttribute("href", "/scaledobjects?status=attention");
  });

  it("shows an API error state", async () => {
    vi.stubGlobal("fetch", vi.fn(() => Promise.resolve(new Response(JSON.stringify({ error: "cluster unavailable" }), { status: 500, headers: { "Content-Type": "application/json" } }))));
    render(<MemoryRouter><App /></MemoryRouter>);
    expect(await screen.findByText("Unable to load data")).toBeInTheDocument();
    expect(screen.getByText("cluster unavailable")).toBeInTheDocument();
  });

  it("renders a null zero-resource response without crashing", async () => {
    vi.stubGlobal("fetch", vi.fn((input: RequestInfo | URL) => {
      const body = input.toString().includes("/meta")
        ? { kedaInstalled: true, prometheusEnabled: false, cluster: "empty" }
        : { items: null, total: null, namespaces: null };
      return Promise.resolve(new Response(JSON.stringify(body), { status: 200, headers: { "Content-Type": "application/json" } }));
    }));
    render(<MemoryRouter initialEntries={["/scaledobjects"]}><App /></MemoryRouter>);
    expect(await screen.findByText("No resources match these filters")).toBeInTheDocument();
    expect(screen.getByText("0 total")).toBeInTheDocument();
    expect(screen.getByRole("combobox", { name: "Namespace" })).toBeInTheDocument();
  });
});
