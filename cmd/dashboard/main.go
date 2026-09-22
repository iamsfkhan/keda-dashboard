package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/iamsfkhan/keda-dashboard/internal/demo"
	"github.com/iamsfkhan/keda-dashboard/internal/kube"
	"github.com/iamsfkhan/keda-dashboard/internal/prometheus"
	"github.com/iamsfkhan/keda-dashboard/internal/server"
	"github.com/iamsfkhan/keda-dashboard/internal/web"
)

func main() {
	level := slog.LevelInfo
	if os.Getenv("LOG_LEVEL") == "debug" {
		level = slog.LevelDebug
	}
	log := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(log)

	demoMode := os.Getenv("DEMO_MODE") == "true"
	cluster, err := newCluster(demoMode, log)
	if err != nil {
		log.Error("startup failed", "error", err)
		os.Exit(1)
	}
	prometheusURL := os.Getenv("PROMETHEUS_URL")
	if demoMode {
		prometheusURL = ""
	}
	metrics := prometheus.New(prometheusURL)
	handler := server.New(cluster, metrics, log, web.Assets())
	address := env("LISTEN_ADDRESS", ":8080")
	httpServer := &http.Server{
		Addr: address, Handler: handler, ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second,
	}

	errs := make(chan error, 1)
	go func() {
		log.Info("server listening", "address", address, "prometheus_enabled", metrics.Enabled())
		errs <- httpServer.ListenAndServe()
	}()

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	select {
	case sig := <-signals:
		log.Info("shutdown requested", "signal", sig.String())
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			log.Error("server stopped unexpectedly", "error", err)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Error("graceful shutdown failed", "error", err)
		_ = httpServer.Close()
	}
}

func newCluster(demoMode bool, log *slog.Logger) (server.Cluster, error) {
	if demoMode {
		log.Info("demo mode enabled", "kubernetes_access", false)
		return demo.New(), nil
	}
	return kube.New(log)
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
