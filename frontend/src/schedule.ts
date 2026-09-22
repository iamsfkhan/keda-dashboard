import type { Resource, TriggerSchedule } from "./api";

export type ScalingExplanation = {
  windowActive: boolean;
  coolingDown: boolean;
  scheduleLabel: string;
  timezone: string;
  explanation: string;
};

type Clock = { minute: number; hour: number; day: number; month: number; dow: number };

const WEEKDAYS = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"];

export function describeScaling(resource: Resource, now = new Date()): ScalingExplanation {
  const schedule = resource.triggers.find((trigger) => trigger.type === "cron" && trigger.schedule?.start && trigger.schedule.end)?.schedule;
  const timezone = schedule?.timezone || "UTC";
  const windowActive = schedule ? cronWindowActive(schedule, now) : false;
  const cooldownUntil = cooldownEnd(resource);
  const coolingDown = Boolean(cooldownUntil && now < cooldownUntil && !windowActive);
  const replicas = replicaLabel(resource.currentReplicas);
  let explanation = `This service is running ${replicas}. KEDA currently wants ${replicaLabel(resource.desiredReplicas)}.`;

  if (schedule && windowActive) {
    explanation = `The cron schedule is active from ${formatCron(schedule.start)} to ${formatCron(schedule.end)} ${timezone}. It is requesting ${replicaLabel(schedule.desiredReplicas || resource.desiredReplicas)} until ${formatCron(schedule.end)}.`;
  } else if (schedule && coolingDown && cooldownUntil) {
    const minutes = Math.round((resource.cooldownSeconds || 0) / 60);
    explanation = `The cron schedule ended at ${formatCron(schedule.end)} ${timezone}. KEDA is keeping ${replicas} during a ${minutes}-minute cooldown until ${formatInZone(cooldownUntil, timezone)}.`;
  } else if (schedule) {
    explanation = `The cron schedule ${formatCron(schedule.start)}–${formatCron(schedule.end)} ${timezone} has ended, so it is not requesting replicas.`;
    if (resource.currentReplicas > 0) explanation += ` This service is still running ${replicas}.`;
  }

  return {
    windowActive,
    coolingDown,
    scheduleLabel: schedule ? `${formatCron(schedule.start)}–${formatCron(schedule.end)}` : "Trigger",
    timezone,
    explanation,
  };
}

export function cronWindowActive(schedule: TriggerSchedule, now: Date): boolean {
  if (!schedule.start || !schedule.end) return false;
  const timezone = schedule.timezone || "UTC";
  const previousStart = previousCron(schedule.start, now, timezone);
  const previousEnd = previousCron(schedule.end, now, timezone);
  return Boolean(previousStart && previousEnd && previousStart.getTime() > previousEnd.getTime());
}

function cooldownEnd(resource: Resource): Date | null {
  if (!resource.lastActiveTime || !resource.cooldownSeconds) return null;
  const lastActive = new Date(resource.lastActiveTime);
  if (Number.isNaN(lastActive.getTime())) return null;
  return new Date(lastActive.getTime() + resource.cooldownSeconds * 1000);
}

function previousCron(expression: string, now: Date, timeZone: string): Date | null {
  const cursor = new Date(now);
  cursor.setSeconds(0, 0);
  const limit = cursor.getTime() - 14 * 24 * 60 * 60 * 1000;
  for (let time = cursor.getTime(); time >= limit; time -= 60_000) {
    const candidate = new Date(time);
    if (cronMatches(expression, zonedClock(candidate, timeZone))) return candidate;
  }
  return null;
}

function cronMatches(expression: string, clock: Clock): boolean {
  const fields = expression.trim().split(/\s+/);
  if (fields.length !== 5) return false;
  const [minute, hour, day, month, dow] = fields;
  const dayRestricted = day !== "*";
  const dowRestricted = dow !== "*";
  const dayMatches = fieldMatches(day, clock.day, 1, 31);
  const dowMatches = fieldMatches(dow, clock.dow, 0, 7) || (clock.dow === 0 && fieldMatches(dow, 7, 0, 7));
  const dateMatches = dayRestricted && dowRestricted ? dayMatches || dowMatches : (!dayRestricted || dayMatches) && (!dowRestricted || dowMatches);
  return fieldMatches(minute, clock.minute, 0, 59) && fieldMatches(hour, clock.hour, 0, 23) && fieldMatches(month, clock.month, 1, 12) && dateMatches;
}

function fieldMatches(field: string, value: number, min: number, max: number): boolean {
  return field.split(",").some((part) => {
    const [range = "*", stepText] = part.split("/");
    const step = stepText ? Number(stepText) : 1;
    if (!Number.isInteger(step) || step <= 0) return false;
    let start = min;
    let end = max;
    if (range !== "*") {
      const [from, to] = range.split("-");
      start = Number(from);
      end = to == null ? start : Number(to);
      if (!Number.isInteger(start) || !Number.isInteger(end)) return false;
    }
    return value >= start && value <= end && (value - start) % step === 0;
  });
}

function zonedClock(date: Date, timeZone: string): Clock {
  const parts = new Intl.DateTimeFormat("en-US", {
    timeZone, hourCycle: "h23", weekday: "short", year: "numeric", month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit",
  }).formatToParts(date);
  const value = (type: string) => parts.find((part) => part.type === type)?.value || "";
  let hour = Number(value("hour"));
  if (hour === 24) hour = 0;
  return { minute: Number(value("minute")), hour, day: Number(value("day")), month: Number(value("month")), dow: Math.max(0, WEEKDAYS.indexOf(value("weekday"))) };
}

function formatCron(expression = ""): string {
  const [minute, hour] = expression.split(/\s+/);
  if (/^\d+$/.test(minute || "") && /^\d+$/.test(hour || "")) return `${hour.padStart(2, "0")}:${minute.padStart(2, "0")}`;
  return expression;
}

function formatInZone(date: Date, timeZone: string): string {
  return new Intl.DateTimeFormat("en-US", { timeZone, hour: "2-digit", minute: "2-digit", hourCycle: "h23" }).format(date);
}

function replicaLabel(count = 0): string {
  return `${count} replica${count === 1 ? "" : "s"}`;
}
