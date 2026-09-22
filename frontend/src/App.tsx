import { useCallback, useEffect, useRef, useState } from "react";
import { Link, Navigate, Route, Routes, useParams, useSearchParams } from "react-router-dom";
import {
  getJSON,
  type Meta,
  type MetricPoint,
  type Overview,
  type Resource,
  type ResourceList,
} from "./api";
import ScalingFlow from "./ScalingFlow";

const EMPTY_OVERVIEW: Overview = {
  scaledObjects: 0,
  scaledJobs: 0,
  ready: 0,
  active: 0,
  paused: 0,
  unhealthy: 0,
  triggerTypes: {},
  namespaces: [],
};

const EMPTY_LIST: ResourceList = { items: [], total: 0, namespaces: [] };

function normalizeOverview(value: Overview | null | undefined): Overview {
  return {
    scaledObjects: Number(value?.scaledObjects) || 0,
    scaledJobs: Number(value?.scaledJobs) || 0,
    ready: Number(value?.ready) || 0,
    active: Number(value?.active) || 0,
    paused: Number(value?.paused) || 0,
    unhealthy: Number(value?.unhealthy) || 0,
    triggerTypes: value?.triggerTypes && typeof value.triggerTypes === "object" ? value.triggerTypes : {},
    namespaces: Array.isArray(value?.namespaces) ? value.namespaces.filter(Boolean) : [],
  };
}

function normalizeResource(value: Resource): Resource {
  return {
    ...value,
    name: value?.name || "Unnamed resource",
    namespace: value?.namespace || "default",
    currentReplicas: Number(value?.currentReplicas) || 0,
    desiredReplicas: Number(value?.desiredReplicas) || 0,
    triggers: Array.isArray(value?.triggers) ? value.triggers.filter(Boolean) : [],
    conditions: Array.isArray(value?.conditions) ? value.conditions.filter(Boolean) : [],
    events: Array.isArray(value?.events) ? value.events.filter(Boolean) : [],
  };
}

function normalizeOptionalResource(value: Resource | null): Resource | null {
  return value ? normalizeResource(value) : null;
}

function normalizeList(value: ResourceList | null | undefined): ResourceList {
  const items = Array.isArray(value?.items) ? value.items.filter(Boolean).map(normalizeResource) : [];
  return {
    items,
    total: value?.total == null || !Number.isFinite(Number(value.total)) ? items.length : Number(value.total),
    namespaces: Array.isArray(value?.namespaces) ? value.namespaces.filter(Boolean) : [],
  };
}

function normalizeMetrics(value: MetricPoint[] | null): MetricPoint[] {
  return Array.isArray(value) ? value : [];
}

const identity = <T,>(value: T) => value;

function useRemote<T>(url: string, initial: T, normalize: (value: T) => T = identity) {
  const [data, setData] = useState(initial);
  const initialRef = useRef(initial);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const load = useCallback(
    async (background = false) => {
      if (!url) {
        setLoading(false);
        return;
      }
      if (!background) setLoading(true);
      try {
        const value = await getJSON<T>(url);
        setData(value == null ? initialRef.current : normalize(value));
        setError("");
      } catch (value) {
        setError(value instanceof Error ? value.message : "Something went wrong");
      } finally {
        setLoading(false);
      }
    },
    [normalize, url],
  );
  useEffect(() => {
    if (!url) {
      setLoading(false);
      return;
    }
    void load();
    const timer = window.setInterval(() => void load(true), 30_000);
    return () => window.clearInterval(timer);
  }, [load, url]);
  return { data, loading, error, reload: load };
}

function StatusPill({ ok, children }: { ok: boolean; children: React.ReactNode }) {
  return <span className={`pill ${ok ? "good" : "bad"}`}>{children}</span>;
}

function Shell() {
  const meta = useRemote<Meta>("/api/v1/meta", {
    kedaInstalled: false,
    prometheusEnabled: false,
    cluster: "",
  });
  return (
    <div className="app">
      <header>
        <Link className="brand" to="/">
          <span className="mark">K</span>
          <span>KEDA <b>Dashboard</b></span>
        </Link>
        <div className="cluster">
          <span className={`dot ${meta.data.kedaInstalled ? "online" : ""}`} />
          {meta.loading ? "Connecting…" : meta.data.demoMode ? "Demo data · no cluster access" : meta.data.kedaInstalled ? `KEDA API ${meta.data.kedaVersion || "detected"}` : "KEDA not detected"}
        </div>
      </header>
      <main>
        <Routes>
          <Route path="/" element={<OverviewPage />} />
          <Route path="/:kind" element={<ResourcePage />} />
          <Route path="/:kind/:namespace/:name" element={<DetailPage prometheus={meta.data.prometheusEnabled} />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </main>
      <footer>Read-only by design · No Secret access</footer>
    </div>
  );
}

function OverviewPage() {
  const state = useRemote<Overview>("/api/v1/overview", EMPTY_OVERVIEW, normalizeOverview);
  const reload = state.reload;
  useEffect(() => {
    const stream = new EventSource("/api/v1/events");
    stream.addEventListener("update", () => void reload(true));
    return () => stream.close();
  }, [reload]);
  if (state.loading) return <PageSkeleton />;
  if (state.error) return <ErrorState message={state.error} retry={() => void state.reload()} />;
  const cards = [
    ["ScaledObjects", state.data.scaledObjects, "/scaledobjects", "neutral"],
    ["ScaledJobs", state.data.scaledJobs, "/scaledjobs", "neutral"],
    ["Active", state.data.active, "/scaledobjects?status=active", "green"],
    ["Needs attention", state.data.unhealthy, "/scaledobjects?status=attention", "orange"],
  ] as const;
  return (
    <>
      <div className="hero">
        <div>
          <p className="eyebrow">CLUSTER OVERVIEW</p>
          <h1>Autoscaling, at a glance.</h1>
          <p>Monitor every KEDA workload and its health without changing your cluster.</p>
        </div>
        <div className="live"><span /> Live updates</div>
      </div>
      <section className="stat-grid">
        {cards.map(([label, value, href, tone]) => {
          const content = <><span className={`stat-icon ${tone}`}>{label[0]}</span><strong>{value}</strong><small>{label}</small></>;
          return href ? <Link className="stat" to={href} key={label}>{content}</Link> : <div className="stat" key={label}>{content}</div>;
        })}
      </section>
      <section className="grid-two">
        <div className="panel">
          <div className="panel-title"><h2>Health</h2><span>{state.data.ready + state.data.unhealthy} resources</span></div>
          <div className="health-row"><span className="health-score">{state.data.ready}</span><div><b>Ready</b><small>{state.data.paused} paused</small></div></div>
          <div className="bar"><i style={{ width: `${Math.round((state.data.ready / Math.max(1, state.data.ready + state.data.unhealthy)) * 100)}%` }} /></div>
        </div>
        <div className="panel">
          <div className="panel-title"><h2>Trigger types</h2><span>{Object.keys(state.data.triggerTypes).length} types</span></div>
          <div className="trigger-cloud">
            {Object.entries(state.data.triggerTypes).length ? Object.entries(state.data.triggerTypes).sort((a, b) => b[1] - a[1]).map(([type, count]) => <Link to={`/scaledobjects?q=${encodeURIComponent(type)}`} key={type}><b>{type}</b><span>{count}</span></Link>) : <Empty compact text="No triggers discovered" />}
          </div>
        </div>
      </section>
    </>
  );
}

function ResourcePage() {
  const { kind = "scaledobjects" } = useParams();
  const title = kind === "scaledjobs" ? "ScaledJobs" : "ScaledObjects";
  const [params, setParams] = useSearchParams();
  const namespace = params.get("namespace") || "";
  const search = params.get("q") || "";
  const status = params.get("status") || "";
  const updateFilter = (key: string, value: string) => {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value);
    else next.delete(key);
    setParams(next, { replace: true });
  };
  const state = useRemote<ResourceList>(`/api/v1/${kind}?${params}`, EMPTY_LIST, normalizeList);
  return (
    <>
      <div className="page-heading">
        <div><p className="eyebrow">KEDA RESOURCES</p><h1>{title}</h1><p>Inspect scaling configuration and operational health.</p></div>
        <span className="count">{state.data.total} total</span>
      </div>
      <div className="toolbar">
        <label className="search"><span>⌕</span><input aria-label="Search resources" placeholder="Search name or trigger…" value={search} onChange={(e) => updateFilter("q", e.target.value)} /></label>
        <select aria-label="Namespace" value={namespace} onChange={(e) => updateFilter("namespace", e.target.value)}>
          <option value="">All namespaces</option>
          {state.data.namespaces.map((ns) => <option key={ns}>{ns}</option>)}
        </select>
        <select aria-label="Status" value={status} onChange={(e) => updateFilter("status", e.target.value)}>
          <option value="">Any status</option>
          <option value="active">Active</option>
          <option value="attention">Needs attention</option>
          <option value="ready">Ready</option>
          <option value="paused">Paused</option>
        </select>
        <nav><Link className={kind === "scaledobjects" ? "active" : ""} to={`/scaledobjects?${params}`}>Objects</Link><Link className={kind === "scaledjobs" ? "active" : ""} to={`/scaledjobs?${params}`}>Jobs</Link></nav>
      </div>
      {state.loading ? <TableSkeleton /> : state.error ? <ErrorState message={state.error} retry={() => void state.reload()} /> : state.data.items.length ? (
        <div className="resource-table">
          <div className="table-head"><span>Resource</span><span>Status</span><span>Replicas</span><span>Triggers</span><span /></div>
          {state.data.items.map((resource) => <ResourceRow resource={resource} kind={kind} key={`${resource.namespace}/${resource.name}`} />)}
        </div>
      ) : <Empty text="No resources match these filters" />}
    </>
  );
}

function ResourceRow({ resource, kind }: { resource: Resource; kind: string }) {
  return (
    <Link className="resource-row" to={`/${kind}/${resource.namespace}/${resource.name}`}>
      <span className="resource-name"><i>{resource.name.slice(0, 2).toUpperCase()}</i><span><b>{resource.name}</b><small>{resource.namespace}</small></span></span>
      <span><StatusPill ok={resource.ready}>{resource.paused ? "Paused" : resource.ready ? "Ready" : "Not ready"}</StatusPill></span>
      <span className="replicas"><b>{resource.currentReplicas}</b> / {resource.desiredReplicas}</span>
      <span className="mini-triggers">{resource.triggers.slice(0, 3).map((trigger) => <em key={trigger.type}>{trigger.type}</em>)}</span>
      <span className="arrow">›</span>
    </Link>
  );
}

function DetailPage({ prometheus }: { prometheus: boolean }) {
  const { kind = "scaledobjects", namespace = "", name = "" } = useParams();
  const state = useRemote<Resource | null>(`/api/v1/${kind}/${namespace}/${name}`, null, normalizeOptionalResource);
  const metrics = useRemote<MetricPoint[] | null>(prometheus ? `/api/v1/metrics/${kind}/${namespace}/${name}` : "", [], normalizeMetrics);
  const [tab, setTab] = useState<"overview" | "events" | "yaml">("overview");
  if (state.loading) return <PageSkeleton />;
  if (state.error || !state.data) return <ErrorState message={state.error || "Resource not found"} retry={() => void state.reload()} />;
  const r = state.data;
  return (
    <>
      <Link className="back" to={`/${kind}`}>← Back to {kind === "scaledjobs" ? "ScaledJobs" : "ScaledObjects"}</Link>
      <div className="detail-head">
        <div><div className="title-line"><span className="large-icon">{r.name.slice(0, 2).toUpperCase()}</span><div><h1>{r.name}</h1><p>{r.namespace} · {r.kind}</p></div></div></div>
        <div><StatusPill ok={r.ready}>{r.paused ? "Paused" : r.ready ? "Ready" : "Not ready"}</StatusPill></div>
      </div>
      <div className="tabs"><button className={tab === "overview" ? "active" : ""} onClick={() => setTab("overview")}>Overview</button><button className={tab === "events" ? "active" : ""} onClick={() => setTab("events")}>Events <span>{r.events?.length || 0}</span></button><button className={tab === "yaml" ? "active" : ""} onClick={() => setTab("yaml")}>Safe YAML</button></div>
      {tab === "overview" && <DetailOverview resource={r} metrics={metrics.data} prometheus={prometheus} />}
      {tab === "events" && <Events resource={r} />}
      {tab === "yaml" && <pre className="yaml">{r.yaml}</pre>}
    </>
  );
}

function DetailOverview({ resource: r, metrics, prometheus }: { resource: Resource; metrics: MetricPoint[] | null; prometheus: boolean }) {
  metrics = metrics || [];
  const max = Math.max(...metrics.map((p) => p.value), 1);
  return (
    <div className="detail-grid">
      <div className="stack">
        <ScalingFlow resource={r} />
        {prometheus && <section className="panel"><div className="panel-title"><h2>Metric history</h2><span>Last hour</span></div>{metrics.length ? <div className="chart">{metrics.map((point) => <i title={`${point.value}`} key={point.timestamp} style={{ height: `${Math.max(4, point.value / max * 100)}%` }} />)}</div> : <Empty compact text="No metric samples returned" />}</section>}
        <section className="panel"><div className="panel-title"><h2>Conditions</h2></div><div className="conditions">{r.conditions.length ? r.conditions.map((c) => <div key={c.type}><StatusPill ok={c.status === "True"}>{c.status}</StatusPill><span><b>{c.type}</b><small>{c.message || c.reason || "No details reported"}</small></span></div>) : <Empty compact text="No conditions reported" />}</div></section>
      </div>
      <section className="panel trigger-panel"><div className="panel-title"><h2>Triggers</h2><span>{r.triggers.length}</span></div>{r.triggers.length ? r.triggers.map((trigger, index) => <div className="trigger-card" key={`${trigger.type}-${index}`}><span>{index + 1}</span><div><b>{trigger.type}</b><small>{trigger.authenticationRef ? `${trigger.authenticationRef.kind}: ${trigger.authenticationRef.name}` : "Scaler trigger"}</small></div><StatusPill ok={trigger.healthy !== false}>{trigger.healthy === false ? "Unhealthy" : "Configured"}</StatusPill></div>) : <Empty compact text="No triggers configured" />}</section>
    </div>
  );
}

function Events({ resource }: { resource: Resource }) {
  return <section className="panel events">{resource.events?.length ? resource.events.map((event, index) => <div key={`${event.timestamp || "event"}-${index}`}><span className={`event-dot ${(event.type || "normal").toLowerCase()}`} /><div><b>{event.reason || "Kubernetes event"}</b><p>{event.message || "No message reported"}</p><small>{event.timestamp ? new Date(event.timestamp).toLocaleString() : "Unknown time"} · {Number(event.count) || 0} occurrence{event.count === 1 ? "" : "s"}</small></div></div>) : <Empty text="No Kubernetes events found" />}</section>;
}

function Empty({ text, compact = false }: { text: string; compact?: boolean }) {
  return <div className={`empty ${compact ? "compact" : ""}`}><span>◇</span><p>{text}</p></div>;
}
function ErrorState({ message, retry }: { message: string; retry: () => void }) {
  return <div className="error-state"><span>!</span><h2>Unable to load data</h2><p>{message}</p><button onClick={retry}>Try again</button></div>;
}
function PageSkeleton() { return <div className="skeleton page"><i /><i /><i /><i /></div>; }
function TableSkeleton() { return <div className="skeleton table"><i /><i /><i /><i /></div>; }

export default Shell;
