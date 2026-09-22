package kube

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/iamsfkhan/keda-dashboard/internal/model"
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

var (
	scaledObjects = schema.GroupVersionResource{Group: "keda.sh", Version: "v1alpha1", Resource: "scaledobjects"}
	scaledJobs    = schema.GroupVersionResource{Group: "keda.sh", Version: "v1alpha1", Resource: "scaledjobs"}
	hpas          = schema.GroupVersionResource{Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"}
	events        = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "events"}
)

type Client struct {
	dynamic   dynamic.Interface
	discovery discovery.DiscoveryInterface
	log       *slog.Logger
	cluster   string
}

func New(log *slog.Logger) (*Client, error) {
	cfg, source, err := config()
	if err != nil {
		return nil, err
	}
	cfg.UserAgent = "keda-dashboard"
	cfg.Timeout = 15 * time.Second
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create kubernetes client: %w", err)
	}
	disc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("create discovery client: %w", err)
	}
	log.Info("kubernetes client configured", "source", source, "host", cfg.Host)
	return &Client{dynamic: dyn, discovery: disc, log: log, cluster: source}, nil
}

func config() (*rest.Config, string, error) {
	if cfg, err := rest.InClusterConfig(); err == nil {
		return cfg, "in-cluster", nil
	}
	path := os.Getenv("KUBECONFIG")
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, "", fmt.Errorf("resolve home directory: %w", err)
		}
		path = filepath.Join(home, ".kube", "config")
	}
	rules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: path}
	overrides := &clientcmd.ConfigOverrides{}
	clientCfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides)
	cfg, err := clientCfg.ClientConfig()
	if err != nil {
		return nil, "", fmt.Errorf("load kubeconfig %q: %w", path, err)
	}
	raw, _ := clientCfg.RawConfig()
	return cfg, "kubeconfig:" + raw.CurrentContext, nil
}

func (c *Client) Meta(ctx context.Context, prometheus bool) model.Meta {
	out := model.Meta{Cluster: c.cluster, PrometheusEnabled: prometheus}
	resources, err := c.discovery.ServerResourcesForGroupVersion("keda.sh/v1alpha1")
	if err != nil {
		return out
	}
	for _, r := range resources.APIResources {
		if r.Name == "scaledobjects" {
			out.KEDAInstalled = true
			break
		}
	}
	if groups, err := c.discovery.ServerGroups(); err == nil {
		for _, group := range groups.Groups {
			if group.Name == "keda.sh" {
				out.KEDAVersion = group.PreferredVersion.Version
			}
		}
	}
	return out
}

func (c *Client) Ready(ctx context.Context) error {
	_, err := c.discovery.ServerVersion()
	return err
}

func (c *Client) List(ctx context.Context, kind, namespace, query, status string) (model.ResourceList, error) {
	gvr, canonical, err := resourceFor(kind)
	if err != nil {
		return model.ResourceList{}, err
	}
	list, err := c.dynamic.Resource(gvr).Namespace(namespaceOrAll(namespace)).List(ctx, metav1.ListOptions{})
	if err != nil {
		return model.ResourceList{}, fmt.Errorf("list %s: %w", canonical, err)
	}
	result := model.ResourceList{
		Items:      make([]model.Resource, 0, len(list.Items)),
		Namespaces: []string{},
	}
	nsSet := map[string]struct{}{}
	query = strings.ToLower(strings.TrimSpace(query))
	hpasByName, hpasByOwner := c.indexHPAs(ctx, namespace)
	for i := range list.Items {
		obj := &list.Items[i]
		item := summarize(obj, canonical)
		if view, ok := matchHPA(obj, hpasByName, hpasByOwner); ok {
			applyHPAView(&item, view)
		}
		nsSet[item.Namespace] = struct{}{}
		if query != "" && !strings.Contains(strings.ToLower(item.Name+" "+item.Namespace+" "+strings.Join(triggerTypes(item.Triggers), " ")), query) {
			continue
		}
		if !statusMatches(item, status) {
			continue
		}
		result.Items = append(result.Items, item)
	}
	sort.Slice(result.Items, func(i, j int) bool {
		if result.Items[i].Namespace == result.Items[j].Namespace {
			return result.Items[i].Name < result.Items[j].Name
		}
		return result.Items[i].Namespace < result.Items[j].Namespace
	})
	for ns := range nsSet {
		result.Namespaces = append(result.Namespaces, ns)
	}
	sort.Strings(result.Namespaces)
	result.Total = len(result.Items)
	return result, nil
}

func (c *Client) Get(ctx context.Context, kind, namespace, name string) (model.Resource, error) {
	gvr, canonical, err := resourceFor(kind)
	if err != nil {
		return model.Resource{}, err
	}
	obj, err := c.dynamic.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return model.Resource{}, fmt.Errorf("get %s %s/%s: %w", canonical, namespace, name, err)
	}
	out := summarize(obj, canonical)
	out.Events = c.relatedEvents(ctx, obj)
	c.attachHPA(ctx, obj, &out)
	c.enrichTarget(ctx, &out)
	safe := sanitize(obj.Object)
	yamlBytes, err := yaml.Marshal(safe)
	if err != nil {
		return model.Resource{}, fmt.Errorf("render safe yaml: %w", err)
	}
	out.YAML = string(yamlBytes)
	return out, nil
}

func (c *Client) Overview(ctx context.Context) (model.Overview, error) {
	objects, err := c.List(ctx, "scaledobjects", "", "", "")
	if err != nil {
		return model.Overview{}, err
	}
	jobs, err := c.List(ctx, "scaledjobs", "", "", "")
	if err != nil {
		return model.Overview{}, err
	}
	out := model.Overview{
		ScaledObjects: len(objects.Items),
		ScaledJobs:    len(jobs.Items),
		TriggerTypes:  map[string]int{},
		Namespaces:    []string{},
	}
	ns := map[string]struct{}{}
	for _, item := range append(objects.Items, jobs.Items...) {
		ns[item.Namespace] = struct{}{}
		if item.Ready {
			out.Ready++
		} else {
			out.Unhealthy++
		}
		if item.Active {
			out.Active++
		}
		if item.Paused {
			out.Paused++
		}
		for _, trigger := range item.Triggers {
			out.TriggerTypes[trigger.Type]++
		}
	}
	for n := range ns {
		out.Namespaces = append(out.Namespaces, n)
	}
	sort.Strings(out.Namespaces)
	return out, nil
}

func resourceFor(kind string) (schema.GroupVersionResource, string, error) {
	switch strings.ToLower(kind) {
	case "scaledobjects", "scaledobject":
		return scaledObjects, "ScaledObject", nil
	case "scaledjobs", "scaledjob":
		return scaledJobs, "ScaledJob", nil
	default:
		return schema.GroupVersionResource{}, "", errors.New("unsupported resource kind")
	}
}

func namespaceOrAll(ns string) string {
	if ns == "" || ns == "all" {
		return metav1.NamespaceAll
	}
	return ns
}

func summarize(obj *unstructured.Unstructured, kind string) model.Resource {
	out := model.Resource{
		Kind: kind, APIVersion: obj.GetAPIVersion(), Namespace: obj.GetNamespace(), Name: obj.GetName(),
		CreatedAt: obj.GetCreationTimestamp().Time, Labels: obj.GetLabels(), Triggers: []model.Trigger{}, Conditions: []model.Condition{},
	}
	annotations := obj.GetAnnotations()
	out.Paused = annotations["autoscaling.keda.sh/paused"] == "true" || annotations["autoscaling.keda.sh/paused-replicas"] != ""
	if current, found := nestedReplica(obj, "currentReplicas", "currentReplicaCount"); found {
		out.CurrentReplicas = current
		out.ReplicaSource = "status"
	}
	if desired, found := nestedReplica(obj, "desiredReplicas", "desiredReplicaCount"); found {
		out.DesiredReplicas = desired
		out.ReplicaSource = "status"
	}
	if minReplicas, found, _ := unstructured.NestedInt64(obj.Object, "spec", "minReplicaCount"); found {
		out.MinReplicas = minReplicas
	}
	if maxReplicas, found, _ := unstructured.NestedInt64(obj.Object, "spec", "maxReplicaCount"); found {
		out.MaxReplicas = maxReplicas
	}
	if cooldown, found, _ := unstructured.NestedInt64(obj.Object, "spec", "cooldownPeriod"); found {
		out.CooldownSeconds = cooldown
	}
	if lastActive, found, _ := unstructured.NestedString(obj.Object, "status", "lastActiveTime"); found {
		out.LastActiveTime = lastActive
	}
	if conditions, ok, _ := unstructured.NestedSlice(obj.Object, "status", "conditions"); ok {
		for _, raw := range conditions {
			m, _ := raw.(map[string]any)
			condition := model.Condition{
				Type: stringValue(m["type"]), Status: stringValue(m["status"]), Reason: stringValue(m["reason"]),
				Message: stringValue(m["message"]), LastTransitionTime: stringValue(m["lastTransitionTime"]),
			}
			out.Conditions = append(out.Conditions, condition)
			if condition.Type == "Ready" {
				out.Ready = condition.Status == "True"
			}
			if condition.Type == "Active" {
				out.Active = condition.Status == "True"
			}
		}
	}
	if triggers, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "triggers"); ok {
		health, _, _ := unstructured.NestedMap(obj.Object, "status", "health")
		activity, _, _ := unstructured.NestedMap(obj.Object, "status", "triggersActivity")
		for index, raw := range triggers {
			m, _ := raw.(map[string]any)
			trigger := model.Trigger{Type: stringValue(m["type"])}
			if auth, ok := m["authenticationRef"].(map[string]any); ok {
				name := stringValue(auth["name"])
				if name != "" {
					kind := stringValue(auth["kind"])
					if kind == "" {
						kind = "TriggerAuthentication"
					}
					namespace := obj.GetNamespace()
					if kind == "ClusterTriggerAuthentication" {
						namespace = ""
					}
					trigger.AuthenticationRef = &model.RelatedResource{Kind: kind, Namespace: namespace, Name: name}
				}
			}
			if meta, ok := m["metadata"].(map[string]any); ok && trigger.Type == "cron" {
				desired, _ := strconv.ParseInt(stringValue(meta["desiredReplicas"]), 10, 64)
				trigger.Schedule = &model.TriggerSchedule{
					Start:           stringValue(meta["start"]),
					End:             stringValue(meta["end"]),
					Timezone:        stringValue(meta["timezone"]),
					DesiredReplicas: desired,
				}
			}
			prefix := fmt.Sprintf("s%d-", index)
			for key, rawHealth := range health {
				if !strings.HasPrefix(key, prefix) {
					continue
				}
				healthMap, _ := rawHealth.(map[string]any)
				healthy := strings.EqualFold(stringValue(healthMap["status"]), "happy")
				trigger.Healthy = &healthy
				break
			}
			for key, rawActivity := range activity {
				if !strings.HasPrefix(key, prefix) {
					continue
				}
				activityMap, _ := rawActivity.(map[string]any)
				if active, ok := boolValue(activityMap["isActive"]); ok {
					trigger.Active = &active
				}
				break
			}
			out.Triggers = append(out.Triggers, trigger)
		}
	}
	if kind == "ScaledObject" {
		name, _, _ := unstructured.NestedString(obj.Object, "spec", "scaleTargetRef", "name")
		targetKind, _, _ := unstructured.NestedString(obj.Object, "spec", "scaleTargetRef", "kind")
		if targetKind == "" {
			targetKind = "Deployment"
		}
		if name != "" {
			out.Target = &model.RelatedResource{Kind: targetKind, Namespace: obj.GetNamespace(), Name: name}
		}
	}
	return out
}

func (c *Client) enrichTarget(ctx context.Context, resource *model.Resource) {
	if resource.Target == nil {
		return
	}
	var gvr schema.GroupVersionResource
	switch strings.ToLower(resource.Target.Kind) {
	case "deployment", "deployments":
		gvr = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}
	case "statefulset", "statefulsets":
		gvr = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}
	case "daemonset", "daemonsets":
		gvr = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}
	default:
		return
	}
	workload, err := c.dynamic.Resource(gvr).Namespace(resource.Namespace).Get(ctx, resource.Target.Name, metav1.GetOptions{})
	if err != nil {
		c.log.Debug("unable to read target workload", "kind", resource.Target.Kind, "name", resource.Target.Name, "error", err)
		return
	}
	ready, _, _ := unstructured.NestedInt64(workload.Object, "status", "readyReplicas")
	replicas, _, _ := unstructured.NestedInt64(workload.Object, "status", "replicas")
	if gvr.Resource == "daemonsets" {
		ready, _, _ = unstructured.NestedInt64(workload.Object, "status", "numberReady")
		replicas, _, _ = unstructured.NestedInt64(workload.Object, "status", "desiredNumberScheduled")
	}
	resource.Target.Ready = fmt.Sprintf("%d/%d ready", ready, replicas)
	if resource.ReplicaSource == "" {
		if gvr.Resource == "daemonsets" {
			resource.CurrentReplicas = ready
			resource.DesiredReplicas = replicas
		} else if specReplicas, found, _ := unstructured.NestedInt64(workload.Object, "spec", "replicas"); found {
			resource.CurrentReplicas = replicas
			resource.DesiredReplicas = specReplicas
		}
		resource.ReplicaSource = "workload"
	}
}

type hpaView struct {
	name      string
	namespace string
	current   int64
	desired   int64
}

func (c *Client) attachHPA(ctx context.Context, obj *unstructured.Unstructured, resource *model.Resource) {
	byName, byOwner := c.indexHPAs(ctx, obj.GetNamespace())
	if view, ok := matchHPA(obj, byName, byOwner); ok {
		applyHPAView(resource, view)
	}
}

func (c *Client) indexHPAs(ctx context.Context, namespace string) (map[string]hpaView, map[string]hpaView) {
	byName := map[string]hpaView{}
	byOwner := map[string]hpaView{}
	list, err := c.dynamic.Resource(hpas).Namespace(namespaceOrAll(namespace)).List(ctx, metav1.ListOptions{})
	if err != nil {
		c.log.Warn("unable to list related HPAs", "namespace", namespace, "error", err)
		return byName, byOwner
	}
	for i := range list.Items {
		hpa := &list.Items[i]
		current, _, _ := unstructured.NestedInt64(hpa.Object, "status", "currentReplicas")
		desired, _, _ := unstructured.NestedInt64(hpa.Object, "status", "desiredReplicas")
		view := hpaView{name: hpa.GetName(), namespace: hpa.GetNamespace(), current: current, desired: desired}
		byName[hpa.GetNamespace()+"/"+hpa.GetName()] = view
		for _, owner := range hpa.GetOwnerReferences() {
			byOwner[string(owner.UID)] = view
		}
	}
	return byName, byOwner
}

func matchHPA(obj *unstructured.Unstructured, byName, byOwner map[string]hpaView) (hpaView, bool) {
	statusName, _, _ := unstructured.NestedString(obj.Object, "status", "hpaName")
	if statusName != "" {
		if view, ok := byName[obj.GetNamespace()+"/"+statusName]; ok {
			return view, true
		}
	}
	if view, ok := byOwner[string(obj.GetUID())]; ok {
		return view, true
	}
	return hpaView{}, false
}

func applyHPAView(resource *model.Resource, view hpaView) {
	resource.HPA = &model.RelatedResource{
		Kind: "HorizontalPodAutoscaler", Namespace: view.namespace, Name: view.name,
		Ready: fmt.Sprintf("%d/%d", view.current, view.desired),
	}
	if resource.ReplicaSource == "status" {
		return
	}
	resource.CurrentReplicas = view.current
	resource.DesiredReplicas = view.desired
	resource.ReplicaSource = "hpa"
}

func nestedReplica(obj *unstructured.Unstructured, names ...string) (int64, bool) {
	for _, name := range names {
		value, found, _ := unstructured.NestedInt64(obj.Object, "status", name)
		if found {
			return value, true
		}
	}
	return 0, false
}

func (c *Client) relatedEvents(ctx context.Context, obj *unstructured.Unstructured) []model.Event {
	list, err := c.dynamic.Resource(events).Namespace(obj.GetNamespace()).List(ctx, metav1.ListOptions{FieldSelector: "involvedObject.uid=" + string(obj.GetUID())})
	if err != nil {
		c.log.Warn("unable to list related events", "resource", obj.GetName(), "error", err)
		return []model.Event{}
	}
	out := make([]model.Event, 0, len(list.Items))
	for i := range list.Items {
		e := &list.Items[i]
		ts := e.GetCreationTimestamp().Time
		if value, ok, _ := unstructured.NestedString(e.Object, "lastTimestamp"); ok {
			if parsed, err := time.Parse(time.RFC3339, value); err == nil {
				ts = parsed
			}
		}
		count, _, _ := unstructured.NestedInt64(e.Object, "count")
		reason, _, _ := unstructured.NestedString(e.Object, "reason")
		message, _, _ := unstructured.NestedString(e.Object, "message")
		eventType, _, _ := unstructured.NestedString(e.Object, "type")
		out = append(out, model.Event{Type: eventType, Reason: reason, Message: message, Count: count, Timestamp: ts})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Timestamp.After(out[j].Timestamp) })
	if len(out) > 50 {
		out = out[:50]
	}
	return out
}

func sanitize(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			lower := strings.ToLower(key)
			if strings.Contains(lower, "password") || strings.Contains(lower, "token") || strings.Contains(lower, "secret") || strings.Contains(lower, "connectionstring") || strings.Contains(lower, "apikey") {
				out[key] = "<redacted>"
			} else {
				out[key] = sanitize(child)
			}
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = sanitize(typed[i])
		}
		return out
	default:
		// Round trips normalize Kubernetes JSON numbers for predictable YAML.
		b, _ := json.Marshal(typed)
		var normalized any
		_ = json.Unmarshal(b, &normalized)
		return normalized
	}
}

func stringValue(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}

func boolValue(v any) (bool, bool) {
	switch typed := v.(type) {
	case bool:
		return typed, true
	case string:
		parsed, err := strconv.ParseBool(typed)
		return parsed, err == nil
	default:
		return false, false
	}
}

func triggerTypes(triggers []model.Trigger) []string {
	out := make([]string, 0, len(triggers))
	for _, trigger := range triggers {
		out = append(out, trigger.Type)
	}
	return out
}

func statusMatches(resource model.Resource, status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
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
