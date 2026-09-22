package server

import (
	"context"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/iamsfkhan/keda-dashboard/internal/demo"
	"github.com/iamsfkhan/keda-dashboard/internal/model"
)

type fakeCluster struct{}

func (fakeCluster) Ready(context.Context) error { return nil }
func (fakeCluster) Meta(context.Context, bool) model.Meta {
	return model.Meta{KEDAInstalled: true}
}
func (fakeCluster) Overview(context.Context) (model.Overview, error) {
	return model.Overview{ScaledObjects: 2, TriggerTypes: map[string]int{}}, nil
}
func (fakeCluster) List(context.Context, string, string, string, string) (model.ResourceList, error) {
	return model.ResourceList{Items: []model.Resource{}, Total: 0, Namespaces: []string{}}, nil
}
func (fakeCluster) Get(context.Context, string, string, string) (model.Resource, error) {
	return model.Resource{Name: "worker", Namespace: "default"}, nil
}

type fakeMetrics struct{}

func (fakeMetrics) Enabled() bool { return false }
func (fakeMetrics) History(context.Context, string, string, string) ([]model.MetricPoint, error) {
	return nil, nil
}

func handler(t *testing.T) http.Handler {
	t.Helper()
	assets := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("dashboard")}}
	return New(fakeCluster{}, fakeMetrics{}, slog.Default(), fs.FS(assets))
}

func TestHealthAndOverview(t *testing.T) {
	for _, tc := range []struct {
		path, contains string
	}{
		{"/healthz", `"status":"ok"`},
		{"/readyz", `"status":"ready"`},
		{"/api/v1/overview", `"scaledObjects":2`},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		res := httptest.NewRecorder()
		handler(t).ServeHTTP(res, req)
		if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), tc.contains) {
			t.Fatalf("%s: status=%d body=%s", tc.path, res.Code, res.Body.String())
		}
	}
}

func TestRejectsUnknownKind(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/v1/secrets", nil)
	res := httptest.NewRecorder()
	handler(t).ServeHTTP(res, req)
	if res.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", res.Code)
	}
}

func TestDemoResourceAPI(t *testing.T) {
	assets := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("dashboard")}}
	demoHandler := New(demo.New(), fakeMetrics{}, slog.Default(), fs.FS(assets))
	for _, tc := range []struct {
		path, contains string
	}{
		{"/api/v1/meta", `"demoMode":true`},
		{"/api/v1/scaledobjects?status=attention", `"name":"shipment-updates"`},
		{"/api/v1/scaledjobs/data/daily-ledger-export", `"kind":"ScaledJob"`},
	} {
		req := httptest.NewRequest(http.MethodGet, tc.path, nil)
		res := httptest.NewRecorder()
		demoHandler.ServeHTTP(res, req)
		if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), tc.contains) {
			t.Fatalf("%s: status=%d body=%s", tc.path, res.Code, res.Body.String())
		}
	}
}

func TestSPAFallback(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/scaledobjects/default/worker", nil)
	res := httptest.NewRecorder()
	handler(t).ServeHTTP(res, req)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "dashboard") {
		t.Fatalf("unexpected SPA response: %d %s", res.Code, res.Body.String())
	}
}
