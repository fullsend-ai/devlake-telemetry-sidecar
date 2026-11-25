package collector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func newTestPoller(t *testing.T, url string) (*PipelinePoller, *prometheus.Registry) {
	t.Helper()
	reg := prometheus.NewRegistry()
	p := NewPipelinePoller(url, &http.Client{Timeout: 2 * time.Second}, 60*time.Second, reg)
	return p, reg
}

func servePipelines(t *testing.T, pipelines []pipeline) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pipelinesResponse{
			Pipelines: pipelines,
			Count:     len(pipelines),
		})
	}))
}

func strPtr(s string) *string { return &s }

func TestPoll_NewTerminalPipelines(t *testing.T) {
	ts := "2025-01-15T10:00:00Z"
	srv := servePipelines(t, []pipeline{
		{ID: 1, Status: dlStatusCompleted, FinishedAt: strPtr(ts)},
		{ID: 2, Status: dlStatusCompleted, FinishedAt: strPtr(ts)},
		{ID: 3, Status: dlStatusFailed, FinishedAt: strPtr(ts)},
	})
	defer srv.Close()

	p, _ := newTestPoller(t, srv.URL)
	p.poll(context.Background())

	if v := testutil.ToFloat64(p.runsTotal.WithLabelValues("success")); v != 2 {
		t.Errorf("expected runs_total{status=success} = 2, got %v", v)
	}
	if v := testutil.ToFloat64(p.runsTotal.WithLabelValues("failed")); v != 1 {
		t.Errorf("expected runs_total{status=failed} = 1, got %v", v)
	}

	expectedTS := float64(time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC).Unix())
	if v := testutil.ToFloat64(p.lastRunTimestamp.WithLabelValues("success")); v != expectedTS {
		t.Errorf("expected last_run_timestamp{status=success} = %v, got %v", expectedTS, v)
	}
}

func TestPoll_DeduplicatesSeenPipelines(t *testing.T) {
	ts := "2025-01-15T10:00:00Z"
	srv := servePipelines(t, []pipeline{
		{ID: 1, Status: dlStatusCompleted, FinishedAt: strPtr(ts)},
	})
	defer srv.Close()

	p, _ := newTestPoller(t, srv.URL)
	p.poll(context.Background())
	p.poll(context.Background())

	if v := testutil.ToFloat64(p.runsTotal.WithLabelValues("success")); v != 1 {
		t.Errorf("expected runs_total{status=success} = 1 after two polls, got %v", v)
	}
}

func TestPoll_RunningPipelinesSkipCounter(t *testing.T) {
	ts := "2025-01-15T10:00:00Z"
	srv := servePipelines(t, []pipeline{
		{ID: 1, Status: dlStatusRunning, BeganAt: strPtr(ts)},
	})
	defer srv.Close()

	p, reg := newTestPoller(t, srv.URL)
	p.poll(context.Background())

	count, err := testutil.GatherAndCount(reg, "devlake_pipeline_runs_total")
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("expected no runs_total series for running pipelines, got %d", count)
	}
}

func TestPoll_NoAuthorizationHeader(t *testing.T) {
	var mu sync.Mutex
	var gotAuth string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		gotAuth = r.Header.Get("Authorization")
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pipelinesResponse{})
	}))
	defer srv.Close()

	p, _ := newTestPoller(t, srv.URL)
	p.poll(context.Background())

	mu.Lock()
	defer mu.Unlock()
	if gotAuth != "" {
		t.Errorf("expected no Authorization header, got %q", gotAuth)
	}
}
