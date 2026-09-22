package demo

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/iamsfkhan/keda-dashboard/internal/model"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// Client is a deterministic, network-free data source for product demos.
type Client struct {
	resources []model.Resource
}

func New() *Client {
	return &Client{resources: demoResources()}
}

func (c *Client) Ready(context.Context) error { return nil }

func (c *Client) Meta(context.Context, bool) model.Meta {
	return model.Meta{
		KEDAInstalled: true,
		KEDAVersion:   "v1alpha1",
		Cluster:       "demo-cluster",
		DemoMode:      true,
	}
}

func (c *Client) Overview(context.Context) (model.Overview, error) {
	out := model.Overview{TriggerTypes: map[string]int{}, Namespaces: []string{}}
	namespaces := map[string]struct{}{}
	for _, resource := range c.resources {
		if resource.Kind == "ScaledJob" {
			out.ScaledJobs++
		} else {
			out.ScaledObjects++
		}
		if resource.Ready {
			out.Ready++
		} else {
			out.Unhealthy++
		}
		if resource.Active {
			out.Active++
		}
		if resource.Paused {
			out.Paused++
		}
		namespaces[resource.Namespace] = struct{}{}
		for _, trigger := range resource.Triggers {
			out.TriggerTypes[trigger.Type]++
		}
	}
	for namespace := range namespaces {
		out.Namespaces = append(out.Namespaces, namespace)
	}
	sort.Strings(out.Namespaces)
	return out, nil
}

func (c *Client) List(_ context.Context, kind, namespace, query, status string) (model.ResourceList, error) {
	canonical, ok := canonicalKind(kind)
	if !ok {
		return model.ResourceList{}, fmt.Errorf("unsupported resource kind")
	}
	out := model.ResourceList{Items: []model.Resource{}, Namespaces: []string{}}
	namespaces := map[string]struct{}{}
	query = strings.ToLower(strings.TrimSpace(query))
	status = strings.ToLower(strings.TrimSpace(status))
	for _, resource := range c.resources {
		if resource.Kind != canonical {
			continue
		}
		namespaces[resource.Namespace] = struct{}{}
		if namespace != "" && namespace != "all" && resource.Namespace != namespace {
			continue
		}
		if query != "" && !resourceMatches(resource, query) {
			continue
		}
		if !statusMatches(resource, status) {
			continue
		}
		out.Items = append(out.Items, resource)
	}
	for value := range namespaces {
		out.Namespaces = append(out.Namespaces, value)
	}
	sort.Strings(out.Namespaces)
	out.Total = len(out.Items)
	return out, nil
}

func (c *Client) Get(_ context.Context, kind, namespace, name string) (model.Resource, error) {
	canonical, ok := canonicalKind(kind)
	if !ok {
		return model.Resource{}, apierrors.NewNotFound(schema.GroupResource{Group: "keda.sh", Resource: kind}, name)
	}
	for _, resource := range c.resources {
		if resource.Kind == canonical && resource.Namespace == namespace && resource.Name == name {
			return resource, nil
		}
	}
	return model.Resource{}, apierrors.NewNotFound(schema.GroupResource{Group: "keda.sh", Resource: kind}, name)
}

func canonicalKind(kind string) (string, bool) {
	switch strings.ToLower(kind) {
	case "scaledobjects", "scaledobject":
		return "ScaledObject", true
	case "scaledjobs", "scaledjob":
		return "ScaledJob", true
	default:
		return "", false
	}
}

func resourceMatches(resource model.Resource, query string) bool {
	values := []string{resource.Name, resource.Namespace}
	for _, trigger := range resource.Triggers {
		values = append(values, trigger.Type)
	}
	return strings.Contains(strings.ToLower(strings.Join(values, " ")), query)
}

func statusMatches(resource model.Resource, status string) bool {
	switch status {
	case "", "all":
		return true
	case "active":
		return resource.Active
	case "attention", "unhealthy":
		return !resource.Ready
	case "paused":
		return resource.Paused
	case "ready":
		return resource.Ready
	default:
		return true
	}
}

func demoResources() []model.Resource {
	created := time.Date(2026, time.September, 1, 9, 0, 0, 0, time.UTC)
	healthy := true
	unhealthy := false
	return []model.Resource{
		{
			Kind: "ScaledObject", APIVersion: "keda.sh/v1alpha1", Namespace: "payments", Name: "checkout-processor",
			CreatedAt: created, Labels: map[string]string{"app": "checkout", "team": "payments"},
			Ready: true, Active: true, CurrentReplicas: 8, DesiredReplicas: 12,
			Target: &model.RelatedResource{Kind: "Deployment", Namespace: "payments", Name: "checkout-processor", Ready: "8/12 ready"},
			HPA:    &model.RelatedResource{Kind: "HorizontalPodAutoscaler", Namespace: "payments", Name: "keda-hpa-checkout-processor", Ready: "8/12"},
			Triggers: []model.Trigger{
				{Type: "kafka", Healthy: &healthy, AuthenticationRef: &model.RelatedResource{Kind: "TriggerAuthentication", Namespace: "payments", Name: "kafka-auth"}},
				{Type: "prometheus", Healthy: &healthy},
			},
			Conditions: []model.Condition{
				{Type: "Ready", Status: "True", Reason: "ScaledObjectReady", Message: "ScaledObject is ready for scaling"},
				{Type: "Active", Status: "True", Reason: "ScalerActive", Message: "Kafka lag is above the activation threshold"},
			},
			Events: []model.Event{
				{Type: "Normal", Reason: "KEDAScaleTargetActivated", Message: "Scaled deployment payments/checkout-processor from 3 to 8 replicas", Count: 1, Timestamp: created.Add(492 * time.Hour)},
			},
			YAML: "apiVersion: keda.sh/v1alpha1\nkind: ScaledObject\nmetadata:\n  name: checkout-processor\n  namespace: payments\nspec:\n  maxReplicaCount: 30\n  scaleTargetRef:\n    name: checkout-processor\n  triggers:\n    - type: kafka\n      metadata:\n        topic: checkout-events\n",
		},
		{
			Kind: "ScaledObject", APIVersion: "keda.sh/v1alpha1", Namespace: "fulfillment", Name: "shipment-updates",
			CreatedAt: created.Add(-72 * time.Hour), Labels: map[string]string{"app": "shipment-updates"},
			Ready: false, Active: false, CurrentReplicas: 1, DesiredReplicas: 1,
			Target:   &model.RelatedResource{Kind: "Deployment", Namespace: "fulfillment", Name: "shipment-updates", Ready: "1/1 ready"},
			Triggers: []model.Trigger{{Type: "aws-sqs-queue", Healthy: &unhealthy, AuthenticationRef: &model.RelatedResource{Kind: "TriggerAuthentication", Namespace: "fulfillment", Name: "sqs-workload-identity"}}},
			Conditions: []model.Condition{
				{Type: "Ready", Status: "False", Reason: "TriggerError", Message: "The scaler is waiting for a valid queue response"},
				{Type: "Active", Status: "False", Reason: "ScalerNotActive", Message: "No metric value is currently available"},
			},
			Events: []model.Event{{Type: "Warning", Reason: "KEDAScalerFailed", Message: "Unable to fetch queue length; retrying with backoff", Count: 3, Timestamp: created.Add(493 * time.Hour)}},
			YAML:   "apiVersion: keda.sh/v1alpha1\nkind: ScaledObject\nmetadata:\n  name: shipment-updates\n  namespace: fulfillment\nspec:\n  scaleTargetRef:\n    name: shipment-updates\n  triggers:\n    - type: aws-sqs-queue\n      authenticationRef:\n        name: sqs-workload-identity\n",
		},
		{
			Kind: "ScaledObject", APIVersion: "keda.sh/v1alpha1", Namespace: "platform", Name: "webhook-dispatcher",
			CreatedAt: created.Add(-240 * time.Hour), Labels: map[string]string{"app": "webhook-dispatcher"},
			Ready: true, Paused: true, CurrentReplicas: 2, DesiredReplicas: 2,
			Target:     &model.RelatedResource{Kind: "Deployment", Namespace: "platform", Name: "webhook-dispatcher", Ready: "2/2 ready"},
			Triggers:   []model.Trigger{{Type: "redis", Healthy: &healthy}},
			Conditions: []model.Condition{{Type: "Ready", Status: "True", Reason: "ScaledObjectReady", Message: "Autoscaling is paused at 2 replicas"}},
			Events:     []model.Event{},
			YAML:       "apiVersion: keda.sh/v1alpha1\nkind: ScaledObject\nmetadata:\n  name: webhook-dispatcher\n  namespace: platform\n  annotations:\n    autoscaling.keda.sh/paused-replicas: \"2\"\n",
		},
		{
			Kind: "ScaledJob", APIVersion: "keda.sh/v1alpha1", Namespace: "data", Name: "daily-ledger-export",
			CreatedAt: created.Add(-336 * time.Hour), Labels: map[string]string{"app": "ledger-export"},
			Ready: true, Active: true, CurrentReplicas: 4, DesiredReplicas: 6,
			Triggers:   []model.Trigger{{Type: "cron", Healthy: &healthy}, {Type: "aws-sqs-queue", Healthy: &healthy}},
			Conditions: []model.Condition{{Type: "Ready", Status: "True", Reason: "ScaledJobReady", Message: "Job scaling configuration is valid"}, {Type: "Active", Status: "True", Reason: "ScalerActive"}},
			Events:     []model.Event{{Type: "Normal", Reason: "ScaledJobActivated", Message: "Created 4 jobs for pending export batches", Count: 4, Timestamp: created.Add(490 * time.Hour)}},
			YAML:       "apiVersion: keda.sh/v1alpha1\nkind: ScaledJob\nmetadata:\n  name: daily-ledger-export\n  namespace: data\nspec:\n  maxReplicaCount: 20\n  triggers:\n    - type: cron\n",
		},
		{
			Kind: "ScaledJob", APIVersion: "keda.sh/v1alpha1", Namespace: "media", Name: "thumbnail-generator",
			CreatedAt: created.Add(-168 * time.Hour), Labels: map[string]string{"app": "thumbnail-generator"},
			Ready: true, CurrentReplicas: 0, DesiredReplicas: 0,
			Triggers:   []model.Trigger{{Type: "rabbitmq", Healthy: &healthy}},
			Conditions: []model.Condition{{Type: "Ready", Status: "True", Reason: "ScaledJobReady", Message: "Waiting for messages"}},
			Events:     []model.Event{},
			YAML:       "apiVersion: keda.sh/v1alpha1\nkind: ScaledJob\nmetadata:\n  name: thumbnail-generator\n  namespace: media\nspec:\n  triggers:\n    - type: rabbitmq\n",
		},
	}
}
