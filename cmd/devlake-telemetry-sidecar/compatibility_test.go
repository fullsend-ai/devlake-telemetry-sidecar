package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestLoadConfigDefaults pins defaults that deployments rely on when a
// variable is unset.
func TestLoadConfigDefaults(t *testing.T) {
	cfg, err := loadConfig(func(string) string { return "" })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	checks := []struct {
		name string
		got  any
		want any
	}{
		{"DevlakeURL", cfg.DevlakeURL, ""},
		{"ProbeInterval", cfg.ProbeInterval, 30 * time.Second},
		{"PipelineInterval", cfg.PipelineInterval, 60 * time.Second},
		{"MetricsPort", cfg.MetricsPort, 9090},
		{"EnablePprof", cfg.EnablePprof, false},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}

// TestLoadConfigOverrides proves documented variables still take precedence
// and that DEVLAKE_URL keeps its single trailing-slash trim.
func TestLoadConfigOverrides(t *testing.T) {
	env := map[string]string{
		"DEVLAKE_URL":                    "http://devlake:8080/api/",
		"PROBE_INTERVAL_SECONDS":         "15",
		"PIPELINE_POLL_INTERVAL_SECONDS": "7",
		"METRICS_PORT":                   "8081",
		"ENABLE_PPROF":                   "true",
	}
	cfg, err := loadConfig(func(k string) string { return env[k] })
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.DevlakeURL != "http://devlake:8080/api" {
		t.Errorf("DevlakeURL = %q, want trailing slash trimmed", cfg.DevlakeURL)
	}
	if cfg.ProbeInterval != 15*time.Second || cfg.PipelineInterval != 7*time.Second {
		t.Errorf("intervals = %v/%v, want 15s/7s", cfg.ProbeInterval, cfg.PipelineInterval)
	}
	if cfg.MetricsPort != 8081 || !cfg.EnablePprof {
		t.Errorf("port/pprof = %d/%v, want 8081/true", cfg.MetricsPort, cfg.EnablePprof)
	}
}

// TestRun_ServesWithDevlakeURLUnset proves the process still starts the server
// and disables the collectors when DEVLAKE_URL is unset.
func TestRun_ServesWithDevlakeURLUnset(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, config{
			ProbeInterval:    30 * time.Second,
			PipelineInterval: 60 * time.Second,
			MetricsPort:      port,
		})
	}()

	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	var resp *http.Response
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		resp, err = http.Get(base + "/health")
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("server did not start: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), `"healthy"`) {
		t.Fatalf("health = %d %s, want 200 healthy", resp.StatusCode, body)
	}

	metrics, err := http.Get(base + "/metrics")
	if err != nil {
		t.Fatalf("metrics: %v", err)
	}
	metricsBody, _ := io.ReadAll(metrics.Body)
	_ = metrics.Body.Close()
	if strings.Contains(string(metricsBody), "devlake_probe_") {
		t.Error("expected collectors to be disabled when DEVLAKE_URL is unset")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run returned %v, want nil after shutdown", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("run did not stop after context cancellation")
	}
}

// TestLoadConfigInvalidValues proves invalid values still fail fast with the
// variable name in the message.
func TestLoadConfigInvalidValues(t *testing.T) {
	tests := []struct{ key, value string }{
		{"PROBE_INTERVAL_SECONDS", "soon"},
		{"PIPELINE_POLL_INTERVAL_SECONDS", "-1x"},
		{"METRICS_PORT", "ninety"},
		{"ENABLE_PPROF", "maybe"},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			_, err := loadConfig(func(k string) string {
				if k == tt.key {
					return tt.value
				}
				return ""
			})
			if err == nil || !strings.Contains(err.Error(), tt.key) {
				t.Fatalf("expected error mentioning %q, got %v", tt.key, err)
			}
		})
	}
}
