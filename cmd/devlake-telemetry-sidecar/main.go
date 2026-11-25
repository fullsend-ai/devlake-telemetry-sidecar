package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/fullsend-ai/devlake-telemetry-sidecar/internal/collector"
	"github.com/fullsend-ai/devlake-telemetry-sidecar/internal/server"
)

type config struct {
	DevlakeURL       string
	ProbeInterval    time.Duration
	PipelineInterval time.Duration
	MetricsPort      int
	EnablePprof      bool
}

func loadConfig(getenv func(string) string) (config, error) {
	probeInterval, err := envDuration(getenv, "PROBE_INTERVAL_SECONDS", 30*time.Second)
	if err != nil {
		return config{}, err
	}
	pipelineInterval, err := envDuration(getenv, "PIPELINE_POLL_INTERVAL_SECONDS", 60*time.Second)
	if err != nil {
		return config{}, err
	}
	metricsPort, err := envInt(getenv, "METRICS_PORT", 9090)
	if err != nil {
		return config{}, err
	}
	enablePprof, err := envBool(getenv, "ENABLE_PPROF", false)
	if err != nil {
		return config{}, err
	}
	return config{
		DevlakeURL:       strings.TrimRight(getenv("DEVLAKE_URL"), "/"),
		ProbeInterval:    probeInterval,
		PipelineInterval: pipelineInterval,
		MetricsPort:      metricsPort,
		EnablePprof:      enablePprof,
	}, nil
}

func main() {
	cfg, err := loadConfig(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := validateConfig(cfg.ProbeInterval, cfg.PipelineInterval, cfg.MetricsPort); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err := run(ctx, cfg); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config) error {
	httpClient := &http.Client{Timeout: 10 * time.Second}

	if cfg.DevlakeURL != "" {
		reg := prometheus.DefaultRegisterer
		prober := collector.NewProber(cfg.DevlakeURL, httpClient, cfg.ProbeInterval, reg)
		pipelinePoller := collector.NewPipelinePoller(cfg.DevlakeURL, httpClient, cfg.PipelineInterval, reg)
		go prober.Run(ctx)
		go pipelinePoller.Run(ctx)
	} else {
		slog.Info("DEVLAKE_URL not set, prometheus collectors disabled")
	}

	slog.Info("starting devlake-telemetry-sidecar",
		"devlake_url", redactURL(cfg.DevlakeURL),
		"port", cfg.MetricsPort,
		"probe_interval", cfg.ProbeInterval,
		"pipeline_interval", cfg.PipelineInterval,
		"pprof", cfg.EnablePprof,
	)

	if cfg.EnablePprof {
		slog.Warn("pprof enabled: /debug/pprof is served unauthenticated on the metrics port; do not expose this port publicly")
	}

	srv := server.New(cfg.MetricsPort, cfg.EnablePprof)
	return srv.Run(ctx)
}

func validateConfig(probeInterval, pipelineInterval time.Duration, port int) error {
	if probeInterval <= 0 {
		return fmt.Errorf("invalid value for PROBE_INTERVAL_SECONDS: must be > 0, got %d", int(probeInterval.Seconds()))
	}
	if pipelineInterval <= 0 {
		return fmt.Errorf("invalid value for PIPELINE_POLL_INTERVAL_SECONDS: must be > 0, got %d", int(pipelineInterval.Seconds()))
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid value for METRICS_PORT: must be 1-65535, got %d", port)
	}
	return nil
}

// redactURL hides any password embedded in the URL so it never reaches logs.
func redactURL(raw string) string {
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "<unparseable URL>"
	}
	return u.Redacted()
}

func envDuration(getenv func(string) string, key string, defaultVal time.Duration) (time.Duration, error) {
	v := getenv(key)
	if v == "" {
		return defaultVal, nil
	}
	sec, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid value for %s: %s", key, v)
	}
	return time.Duration(sec) * time.Second, nil
}

func envInt(getenv func(string) string, key string, defaultVal int) (int, error) {
	v := getenv(key)
	if v == "" {
		return defaultVal, nil
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("invalid value for %s: %s", key, v)
	}
	return i, nil
}

func envBool(getenv func(string) string, key string, defaultVal bool) (bool, error) {
	v := getenv(key)
	if v == "" {
		return defaultVal, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("invalid value for %s: %s", key, v)
	}
	return b, nil
}
