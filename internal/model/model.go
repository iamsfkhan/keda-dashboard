package model

import "time"

type Condition struct {
	Type               string `json:"type"`
	Status             string `json:"status"`
	Reason             string `json:"reason,omitempty"`
	Message            string `json:"message,omitempty"`
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
}

type TriggerSchedule struct {
	Start           string `json:"start,omitempty"`
	End             string `json:"end,omitempty"`
	Timezone        string `json:"timezone,omitempty"`
	DesiredReplicas int64  `json:"desiredReplicas,omitempty"`
}

type Trigger struct {
	Type              string           `json:"type"`
	Healthy           *bool            `json:"healthy,omitempty"`
	Active            *bool            `json:"active,omitempty"`
	Schedule          *TriggerSchedule `json:"schedule,omitempty"`
	AuthenticationRef *RelatedResource `json:"authenticationRef,omitempty"`
}

type RelatedResource struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
	Ready     string `json:"ready,omitempty"`
}

type Event struct {
	Type      string    `json:"type"`
	Reason    string    `json:"reason"`
	Message   string    `json:"message"`
	Count     int64     `json:"count"`
	Timestamp time.Time `json:"timestamp"`
}

type Resource struct {
	Kind            string            `json:"kind"`
	APIVersion      string            `json:"apiVersion"`
	Namespace       string            `json:"namespace"`
	Name            string            `json:"name"`
	CreatedAt       time.Time         `json:"createdAt"`
	Labels          map[string]string `json:"labels,omitempty"`
	Paused          bool              `json:"paused"`
	Ready           bool              `json:"ready"`
	Active          bool              `json:"active"`
	CurrentReplicas int64             `json:"currentReplicas"`
	DesiredReplicas int64             `json:"desiredReplicas"`
	ReplicaSource   string            `json:"replicaSource,omitempty"`
	MinReplicas     int64             `json:"minReplicas,omitempty"`
	MaxReplicas     int64             `json:"maxReplicas,omitempty"`
	CooldownSeconds int64             `json:"cooldownSeconds,omitempty"`
	LastActiveTime  string            `json:"lastActiveTime,omitempty"`
	Target          *RelatedResource  `json:"target,omitempty"`
	HPA             *RelatedResource  `json:"hpa,omitempty"`
	Triggers        []Trigger         `json:"triggers"`
	Conditions      []Condition       `json:"conditions"`
	Events          []Event           `json:"events,omitempty"`
	YAML            string            `json:"yaml,omitempty"`
}

type ResourceList struct {
	Items      []Resource `json:"items"`
	Total      int        `json:"total"`
	Namespaces []string   `json:"namespaces"`
}

type Overview struct {
	ScaledObjects int            `json:"scaledObjects"`
	ScaledJobs    int            `json:"scaledJobs"`
	Ready         int            `json:"ready"`
	Active        int            `json:"active"`
	Paused        int            `json:"paused"`
	Unhealthy     int            `json:"unhealthy"`
	TriggerTypes  map[string]int `json:"triggerTypes"`
	Namespaces    []string       `json:"namespaces"`
}

type Meta struct {
	KEDAInstalled     bool   `json:"kedaInstalled"`
	KEDAVersion       string `json:"kedaVersion,omitempty"`
	PrometheusEnabled bool   `json:"prometheusEnabled"`
	Cluster           string `json:"cluster"`
	DemoMode          bool   `json:"demoMode"`
}

type MetricPoint struct {
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
}
