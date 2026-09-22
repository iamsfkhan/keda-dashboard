package demo

import (
	"context"
	"testing"
)

func TestDemoOverviewIsDeterministic(t *testing.T) {
	client := New()
	overview, err := client.Overview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if overview.ScaledObjects != 3 || overview.ScaledJobs != 2 {
		t.Fatalf("unexpected resource counts: %#v", overview)
	}
	if overview.Active != 2 || overview.Unhealthy != 1 || overview.Paused != 1 {
		t.Fatalf("unexpected status counts: %#v", overview)
	}
	if overview.TriggerTypes["aws-sqs-queue"] != 2 {
		t.Fatalf("unexpected trigger counts: %#v", overview.TriggerTypes)
	}
}

func TestDemoResourceAPIsSupportFiltersAndDetails(t *testing.T) {
	client := New()
	list, err := client.List(context.Background(), "scaledobjects", "", "", "attention")
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 1 || list.Items[0].Name != "shipment-updates" {
		t.Fatalf("unexpected attention list: %#v", list)
	}

	list, err = client.List(context.Background(), "scaledjobs", "data", "cron", "active")
	if err != nil {
		t.Fatal(err)
	}
	if list.Total != 1 || list.Items[0].Name != "daily-ledger-export" {
		t.Fatalf("unexpected filtered jobs: %#v", list)
	}

	resource, err := client.Get(context.Background(), "scaledobjects", "payments", "checkout-processor")
	if err != nil {
		t.Fatal(err)
	}
	if len(resource.Events) == 0 || resource.YAML == "" || len(resource.Conditions) == 0 {
		t.Fatalf("demo detail is incomplete: %#v", resource)
	}
	if !client.Meta(context.Background(), false).DemoMode {
		t.Fatal("demo metadata must identify demo mode")
	}
}
