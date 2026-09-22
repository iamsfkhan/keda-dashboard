import { useState, type CSSProperties } from "react";
import type { Resource } from "./api";
import { describeScaling } from "./schedule";

export default function ScalingFlow({ resource }: { resource: Resource }) {
  const [replay, setReplay] = useState(0);
  const story = describeScaling(resource);
  const count = Math.max(0, resource.currentReplicas);
  const phase = story.windowActive ? "Scheduled" : story.coolingDown ? "Cooldown" : "Schedule ended";

  return (
    <section className="panel">
      <div className="panel-title">
        <h2>Scaling status</h2>
        <span>{resource.active ? "Active" : story.coolingDown ? "Cooldown" : "Idle"}</span>
      </div>
      <div className="replica-big">
        <strong>{resource.currentReplicas}</strong><span>current replicas</span><i>→</i><strong>{resource.desiredReplicas}</strong><span>desired replicas</span>
      </div>
      <div className={`scaling-flow ${story.coolingDown ? "holding" : ""}`} key={replay}>
        <div className="flow-node">
          <small>Cron schedule</small>
          <b>{story.scheduleLabel}</b>
          <span>{story.timezone}</span>
          <em>{phase}</em>
        </div>
        <div className="flow-link" aria-hidden="true" />
        <button className="flow-node service" type="button" onClick={() => setReplay((value) => value + 1)}>
          <small>{resource.target?.kind || "Service"}</small>
          <b>{resource.target?.name || resource.name}</b>
          <span>{resource.target?.ready || "Click to replay"}</span>
        </button>
        <div className="flow-link to-replicas" aria-hidden="true" />
        <div className="flow-node replica-stage">
          <small>Replicas</small>
          {count > 0 ? Array.from({ length: count }, (_, index) => (
            <i key={`${replay}-${index}`} style={{ "--delay": `${0.85 + index * 0.12}s` } as CSSProperties}>Replica {index + 1}</i>
          )) : <i className="zero" style={{ "--delay": ".85s" } as CSSProperties}>Scaled to zero</i>}
        </div>
      </div>
      <p className="story-copy">{story.explanation}</p>
      <p className="replay-hint">Click the service to replay how this schedule connects to the replica count.</p>
      {(resource.target || resource.hpa) && <div className="related">{[resource.target, resource.hpa].filter(Boolean).map((item) => <div key={item!.kind}><small>{item!.kind}</small><b>{item!.name}</b><span>{item!.ready}</span></div>)}</div>}
    </section>
  );
}
