package main

import (
	"io"
	"log/slog"
	"testing"

	"github.com/iamsfkhan/keda-dashboard/internal/demo"
)

func TestDemoModeSelectsNetworkFreeProvider(t *testing.T) {
	cluster, err := newCluster(true, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cluster.(*demo.Client); !ok {
		t.Fatalf("expected demo provider, got %T", cluster)
	}
}
