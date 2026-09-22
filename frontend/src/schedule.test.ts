import { describe, expect, it } from "vitest";
import type { Resource } from "./api";
import { cronWindowActive, describeScaling } from "./schedule";

const schedule = { start: "00 09 * * *", end: "00 18 * * *", timezone: "UTC", desiredReplicas: 1 };

function resource(nowState: Partial<Resource> = {}): Resource {
  return {
    kind: "ScaledObject",
    apiVersion: "keda.sh/v1alpha1",
    namespace: "checkout",
    name: "checkout-api",
    createdAt: "2026-09-21T07:53:43Z",
    paused: false,
    ready: true,
    active: false,
    currentReplicas: 1,
    desiredReplicas: 1,
    cooldownSeconds: 1800,
    lastActiveTime: "2026-09-22T18:00:00Z",
    triggers: [{ type: "cron", schedule }],
    conditions: [],
    ...nowState,
  };
}

describe("cron schedule", () => {
  it("is active during the daily window", () => {
    expect(cronWindowActive(schedule, new Date("2026-09-22T12:00:00Z"))).toBe(true);
  });

  it("ends at the configured time", () => {
    expect(cronWindowActive(schedule, new Date("2026-09-22T18:00:00Z"))).toBe(false);
  });

  it("explains a cooldown that is still holding the replica", () => {
    const story = describeScaling(resource(), new Date("2026-09-22T18:10:00Z"));
    expect(story.windowActive).toBe(false);
    expect(story.coolingDown).toBe(true);
    expect(story.explanation).toContain("30-minute cooldown");
    expect(story.explanation).toContain("1 replica");
  });
});
