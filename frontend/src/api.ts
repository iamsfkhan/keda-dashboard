export type Condition = {
  type: string;
  status: string;
  reason?: string;
  message?: string;
  lastTransitionTime?: string;
};

export type RelatedResource = {
  kind: string;
  namespace?: string;
  name: string;
  ready?: string;
};
export type Trigger = {
  type: string;
  healthy?: boolean;
  authenticationRef?: RelatedResource;
};
export type KedaEvent = {
  type: string;
  reason: string;
  message: string;
  count: number;
  timestamp: string;
};
export type Resource = {
  kind: "ScaledObject" | "ScaledJob";
  apiVersion: string;
  namespace: string;
  name: string;
  createdAt: string;
  labels?: Record<string, string>;
  paused: boolean;
  ready: boolean;
  active: boolean;
  currentReplicas: number;
  desiredReplicas: number;
  target?: RelatedResource;
  hpa?: RelatedResource;
  triggers: Trigger[];
  conditions: Condition[];
  events?: KedaEvent[];
  yaml?: string;
};
export type ResourceList = {
  items: Resource[];
  total: number;
  namespaces: string[];
};
export type Overview = {
  scaledObjects: number;
  scaledJobs: number;
  ready: number;
  active: number;
  paused: number;
  unhealthy: number;
  triggerTypes: Record<string, number>;
  namespaces: string[];
};
export type Meta = {
  kedaInstalled: boolean;
  kedaVersion?: string;
  prometheusEnabled: boolean;
  cluster: string;
  demoMode?: boolean;
};
export type MetricPoint = { timestamp: number; value: number };

export async function getJSON<T>(url: string, signal?: AbortSignal): Promise<T> {
  const response = await fetch(url, { signal, headers: { Accept: "application/json" } });
  if (!response.ok) {
    let message = `Request failed (${response.status})`;
    try {
      const body = (await response.json()) as { error?: string };
      message = body.error || message;
    } catch {
      // Keep the HTTP fallback when the response is not JSON.
    }
    throw new Error(message);
  }
  return response.json() as Promise<T>;
}
