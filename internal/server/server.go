package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/iamsfkhan/keda-dashboard/internal/model"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
)

type Cluster interface {
	Ready(context.Context) error
	Meta(context.Context, bool) model.Meta
	Overview(context.Context) (model.Overview, error)
	List(context.Context, string, string, string, string) (model.ResourceList, error)
	Get(context.Context, string, string, string) (model.Resource, error)
}

type Metrics interface {
	Enabled() bool
	History(context.Context, string, string, string) ([]model.MetricPoint, error)
}

type Server struct {
	cluster Cluster
	metrics Metrics
	log     *slog.Logger
	assets  fs.FS
}

func New(cluster Cluster, metrics Metrics, log *slog.Logger, assets fs.FS) http.Handler {
	s := &Server{cluster: cluster, metrics: metrics, log: log, assets: assets}
	r := chi.NewRouter()
	r.Use(s.recoverer, s.requestLog, securityHeaders)
	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	r.Get("/readyz", s.ready)
	r.Route("/api/v1", func(r chi.Router) {
		r.Get("/meta", s.meta)
		r.Get("/overview", s.overview)
		r.Get("/events", s.stream)
		r.Get("/metrics/{kind}/{namespace}/{name}", s.metricHistory)
		r.Get("/{kind}", s.list)
		r.Get("/{kind}/{namespace}/{name}", s.detail)
	})
	r.Handle("/*", s.spa())
	r.Handle("/", s.spa())
	return r
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if err := s.cluster.Ready(ctx); err != nil {
		jsonError(w, http.StatusServiceUnavailable, "kubernetes API unavailable")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (s *Server) meta(w http.ResponseWriter, r *http.Request) {
	jsonResponse(w, http.StatusOK, s.cluster.Meta(r.Context(), s.metrics.Enabled()))
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	value, err := s.cluster.Overview(r.Context())
	if err != nil {
		s.fail(w, "load overview", err)
		return
	}
	jsonResponse(w, http.StatusOK, value)
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	kind := chi.URLParam(r, "kind")
	if !validKind(kind) {
		jsonError(w, http.StatusNotFound, "resource not found")
		return
	}
	value, err := s.cluster.List(
		r.Context(),
		kind,
		r.URL.Query().Get("namespace"),
		r.URL.Query().Get("q"),
		r.URL.Query().Get("status"),
	)
	if err != nil {
		s.fail(w, "list resources", err)
		return
	}
	jsonResponse(w, http.StatusOK, value)
}

func (s *Server) detail(w http.ResponseWriter, r *http.Request) {
	kind := chi.URLParam(r, "kind")
	if !validKind(kind) {
		jsonError(w, http.StatusNotFound, "resource not found")
		return
	}
	value, err := s.cluster.Get(r.Context(), kind, chi.URLParam(r, "namespace"), chi.URLParam(r, "name"))
	if err != nil {
		s.fail(w, "get resource", err)
		return
	}
	jsonResponse(w, http.StatusOK, value)
}

func (s *Server) metricHistory(w http.ResponseWriter, r *http.Request) {
	if !s.metrics.Enabled() {
		jsonResponse(w, http.StatusOK, []model.MetricPoint{})
		return
	}
	kind := chi.URLParam(r, "kind")
	if !validKind(kind) {
		jsonError(w, http.StatusNotFound, "resource not found")
		return
	}
	points, err := s.metrics.History(r.Context(), chi.URLParam(r, "namespace"), chi.URLParam(r, "name"), kind)
	if err != nil {
		s.log.Warn("prometheus query failed", "error", err)
		jsonResponse(w, http.StatusOK, []model.MetricPoint{})
		return
	}
	jsonResponse(w, http.StatusOK, points)
}

func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		jsonError(w, http.StatusNotImplemented, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	send := func() bool {
		overview, err := s.cluster.Overview(r.Context())
		if err != nil {
			return false
		}
		data, _ := json.Marshal(overview)
		_, _ = fmt.Fprintf(w, "event: update\ndata: %s\n\n", data)
		flusher.Flush()
		return true
	}
	if !send() {
		return
	}
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !send() {
				return
			}
		}
	}
}

func (s *Server) spa() http.Handler {
	files := http.FileServer(http.FS(s.assets))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if clean == "." {
			clean = "index.html"
		}
		if _, err := fs.Stat(s.assets, clean); err != nil {
			r.URL.Path = "/"
		}
		files.ServeHTTP(w, r)
	})
}

func (s *Server) fail(w http.ResponseWriter, operation string, err error) {
	s.log.Error(operation, "error", err)
	if apierrors.IsNotFound(err) {
		jsonError(w, http.StatusNotFound, "resource not found")
		return
	}
	if apierrors.IsForbidden(err) || apierrors.IsUnauthorized(err) {
		jsonError(w, http.StatusForbidden, "kubernetes access denied")
		return
	}
	jsonError(w, http.StatusInternalServerError, operation+" failed")
}

func (s *Server) requestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.log.Info("http request", "method", r.Method, "path", r.URL.Path, "duration_ms", time.Since(start).Milliseconds())
	})
}

func (s *Server) recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				s.log.Error("request panic", "error", recovered)
				jsonError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; connect-src 'self'; img-src 'self' data:")
		next.ServeHTTP(w, r)
	})
}

func jsonResponse(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func jsonError(w http.ResponseWriter, status int, message string) {
	jsonResponse(w, status, map[string]string{"error": message})
}

func validKind(kind string) bool {
	return kind == "scaledobjects" || kind == "scaledjobs"
}
