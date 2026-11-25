package collector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	dto "github.com/prometheus/client_model/go"
)

func TestMetricContract(t *testing.T) {
	reg := prometheus.NewRegistry()
	p := NewProber("http://example.invalid", &http.Client{}, 30*time.Second, reg)
	poll := NewPipelinePoller("http://example.invalid", &http.Client{}, 60*time.Second, reg)

	// Label values must exist before a CounterVec/GaugeVec/HistogramVec
	// exports its metric family.
	p.requestsTotal.WithLabelValues("/health", "200")
	p.durationSeconds.WithLabelValues("/health")
	p.up.WithLabelValues("/health")
	poll.runsTotal.WithLabelValues("success")
	poll.lastRunTimestamp.WithLabelValues("success")

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	gotLabels := make(map[string][]string, len(mfs))
	gotHelp := make(map[string]string, len(mfs))
	gotType := make(map[string]dto.MetricType, len(mfs))
	for _, mf := range mfs {
		labels := []string{}
		if len(mf.Metric) > 0 {
			for _, lp := range mf.Metric[0].Label {
				labels = append(labels, lp.GetName())
			}
		}
		sort.Strings(labels)
		gotLabels[mf.GetName()] = labels
		gotHelp[mf.GetName()] = mf.GetHelp()
		gotType[mf.GetName()] = mf.GetType()
	}

	wantLabels := map[string][]string{
		"devlake_probe_requests_total":        {"endpoint", "status_code"},
		"devlake_probe_duration_seconds":      {"endpoint"},
		"devlake_probe_up":                    {"endpoint"},
		"devlake_pipeline_runs_total":         {"status"},
		"devlake_pipeline_last_run_timestamp": {"status"},
	}
	for name, want := range wantLabels {
		got, ok := gotLabels[name]
		if !ok {
			t.Errorf("metric %q missing", name)
			continue
		}
		if len(got) != len(want) {
			t.Errorf("%s labels = %v, want %v", name, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("%s labels = %v, want %v", name, got, want)
				break
			}
		}
	}

	wantHelp := map[string]string{
		"devlake_probe_requests_total":        "Total number of probe requests against the DevLake API.",
		"devlake_probe_duration_seconds":      "Duration of probe requests against the DevLake API.",
		"devlake_probe_up":                    "Whether the DevLake API endpoint is up (1) or down (0).",
		"devlake_pipeline_runs_total":         "Total number of pipeline runs by terminal status.",
		"devlake_pipeline_last_run_timestamp": "Unix timestamp of the last observed pipeline run by status.",
	}
	for name, want := range wantHelp {
		if got := gotHelp[name]; got != want {
			t.Errorf("%s help = %q, want %q", name, got, want)
		}
	}

	wantType := map[string]dto.MetricType{
		"devlake_probe_requests_total":        dto.MetricType_COUNTER,
		"devlake_probe_duration_seconds":      dto.MetricType_HISTOGRAM,
		"devlake_probe_up":                    dto.MetricType_GAUGE,
		"devlake_pipeline_runs_total":         dto.MetricType_COUNTER,
		"devlake_pipeline_last_run_timestamp": dto.MetricType_GAUGE,
	}
	for name, want := range wantType {
		if got := gotType[name]; got != want {
			t.Errorf("%s type = %v, want %v", name, got, want)
		}
	}
}

func TestProbeEndpoints(t *testing.T) {
	p, _ := newTestProber(t, "http://example.invalid")
	want := []string{"/health", "/ping"}
	if len(p.endpoints) != len(want) {
		t.Fatalf("endpoints = %v, want %v", p.endpoints, want)
	}
	for i := range want {
		if p.endpoints[i] != want[i] {
			t.Errorf("endpoints = %v, want %v", p.endpoints, want)
			break
		}
	}
}

func TestProbe_ContextCancellation(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		<-block
	}))
	defer srv.Close()
	defer close(block)

	p, _ := newTestProber(t, srv.URL)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	p.probe(ctx, "/health")

	if v := testutil.ToFloat64(p.requestsTotal.WithLabelValues("/health", "0")); v != 1 {
		t.Errorf("expected requests_total{status_code=0} = 1, got %v", v)
	}
	if v := testutil.ToFloat64(p.up.WithLabelValues("/health")); v != 0 {
		t.Errorf("expected probe_up = 0, got %v", v)
	}
}

func TestStatusMapContract(t *testing.T) {
	want := map[string]string{
		"TASK_COMPLETED": "success",
		"TASK_FAILED":    "failed",
		"TASK_RUNNING":   "running",
		"TASK_CANCELLED": "cancelled",
	}
	for in, out := range want {
		if statusMap[in] != out {
			t.Errorf("statusMap[%q] = %q, want %q", in, statusMap[in], out)
		}
	}
	if len(statusMap) != len(want) {
		t.Errorf("statusMap has %d entries, want %d", len(statusMap), len(want))
	}
}
