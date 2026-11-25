package main

import (
	"strings"
	"testing"
	"time"
)

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name     string
		probe    time.Duration
		pipeline time.Duration
		port     int
		wantErr  string
	}{
		{"defaults", 30 * time.Second, 60 * time.Second, 9090, ""},
		{"port bounds low", time.Second, time.Second, 1, ""},
		{"port bounds high", time.Second, time.Second, 65535, ""},
		{"zero probe interval", 0, time.Second, 9090, "PROBE_INTERVAL_SECONDS"},
		{"negative probe interval", -time.Second, time.Second, 9090, "PROBE_INTERVAL_SECONDS"},
		{"zero pipeline interval", time.Second, 0, 9090, "PIPELINE_POLL_INTERVAL_SECONDS"},
		{"port zero", time.Second, time.Second, 0, "METRICS_PORT"},
		{"port too high", time.Second, time.Second, 65536, "METRICS_PORT"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfig(tt.probe, tt.pipeline, tt.port)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error mentioning %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestRedactURL(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", ""},
		{"http://devlake:8080/api", "http://devlake:8080/api"},
		{"https://user:s3cret@devlake/api", "https://user:xxxxx@devlake/api"},
		{"://bad", "<unparseable URL>"},
	}
	for _, tt := range tests {
		if got := redactURL(tt.in); got != tt.want {
			t.Errorf("redactURL(%q) = %q, want %q", tt.in, got, tt.want)
		}
		if strings.Contains(redactURL(tt.in), "s3cret") {
			t.Errorf("redactURL(%q) leaked the password", tt.in)
		}
	}
}
