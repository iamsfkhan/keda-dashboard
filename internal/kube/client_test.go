package kube

import (
	"encoding/json"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestSummarizeScaledObject(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "keda.sh/v1alpha1",
		"kind":       "ScaledObject",
		"metadata": map[string]any{
			"name": "worker", "namespace": "default",
			"annotations": map[string]any{"autoscaling.keda.sh/paused": "true"},
		},
		"spec": map[string]any{
			"scaleTargetRef": map[string]any{"name": "worker", "kind": "Deployment"},
			"triggers": []any{map[string]any{
				"type":              "kafka",
				"authenticationRef": map[string]any{"name": "kafka-auth"},
			}},
		},
		"status": map[string]any{
			"currentReplicas": int64(2),
			"desiredReplicas": int64(4),
			"conditions": []any{
				map[string]any{"type": "Ready", "status": "True"},
				map[string]any{"type": "Active", "status": "True"},
			},
			"health": map[string]any{"s0-kafka": map[string]any{"status": "Happy"}},
		},
	}}
	got := summarize(obj, "ScaledObject")
	if !got.Ready || !got.Active || !got.Paused {
		t.Fatalf("unexpected state: %#v", got)
	}
	if got.CurrentReplicas != 2 || got.DesiredReplicas != 4 {
		t.Fatalf("unexpected replicas: %d/%d", got.CurrentReplicas, got.DesiredReplicas)
	}
	if len(got.Triggers) != 1 || got.Triggers[0].Healthy == nil || !*got.Triggers[0].Healthy {
		t.Fatalf("unexpected trigger health: %#v", got.Triggers)
	}
	if got.Triggers[0].AuthenticationRef == nil ||
		got.Triggers[0].AuthenticationRef.Kind != "TriggerAuthentication" ||
		got.Triggers[0].AuthenticationRef.Name != "kafka-auth" {
		t.Fatalf("unexpected authentication reference: %#v", got.Triggers[0].AuthenticationRef)
	}
}

func TestApplyHPAReplicasWhenStatusOmitsCounts(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "checkout-api", "namespace": "checkout", "uid": "abc"},
		"spec": map[string]any{
			"minReplicaCount": int64(0),
			"maxReplicaCount": int64(2),
			"cooldownPeriod":  int64(1800),
			"triggers": []any{map[string]any{
				"type": "cron",
				"metadata": map[string]any{
					"start": "00 09 * * *", "end": "00 18 * * *", "timezone": "UTC", "desiredReplicas": "1",
				},
			}},
		},
		"status": map[string]any{
			"hpaName":        "keda-hpa-api",
			"lastActiveTime": "2026-09-22T17:29:49Z",
			"conditions":     []any{map[string]any{"type": "Active", "status": "False"}},
			"triggersActivity": map[string]any{
				"s0-cron-UTC": map[string]any{"isActive": false},
			},
		},
	}}
	got := summarize(obj, "ScaledObject")
	if got.CurrentReplicas != 0 || got.ReplicaSource != "" || got.CooldownSeconds != 1800 {
		t.Fatalf("unexpected summary: %#v", got)
	}
	if got.Triggers[0].Schedule == nil || got.Triggers[0].Schedule.DesiredReplicas != 1 || got.Triggers[0].Schedule.Timezone != "UTC" {
		t.Fatalf("unexpected schedule: %#v", got.Triggers[0].Schedule)
	}
	if got.Triggers[0].Active == nil || *got.Triggers[0].Active {
		t.Fatalf("unexpected trigger activity: %#v", got.Triggers[0].Active)
	}
	applyHPAView(&got, hpaView{name: "keda-hpa-checkout-api", namespace: "checkout", current: 1, desired: 1})
	if got.CurrentReplicas != 1 || got.DesiredReplicas != 1 || got.ReplicaSource != "hpa" || got.HPA == nil || got.HPA.Ready != "1/1" {
		t.Fatalf("hpa replicas were not applied: %#v", got)
	}
}

func TestSanitizeRedactsNestedCredentialFields(t *testing.T) {
	value := map[string]any{
		"metadata": map[string]any{"name": "safe"},
		"spec": map[string]any{
			"password": "leak",
			"triggers": []any{map[string]any{"metadata": map[string]any{"apiKey": "leak", "queue": "safe"}}},
		},
	}
	rendered := sanitize(value)
	text := strings.ToLower(toJSON(t, rendered))
	if strings.Contains(text, "leak") || !strings.Contains(text, "redacted") || !strings.Contains(text, "safe") {
		t.Fatalf("unexpected sanitized value: %s", text)
	}
}

func toJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
