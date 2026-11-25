package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	dlStatusCompleted = "TASK_COMPLETED"
	dlStatusFailed    = "TASK_FAILED"
	dlStatusRunning   = "TASK_RUNNING"
	dlStatusCancelled = "TASK_CANCELLED"
)

const (
	pipelinesPageSize = 100
	// Upper bound on extra pages fetched in one poll when more pipelines
	// changed than fit on the first page since the previous poll.
	maxCatchUpPages  = 10
	maxResponseBytes = 10 << 20
	// The seen set is only pruned once it exceeds this size, so IDs just
	// below the current window survive and a shifted window can't recount them.
	maxSeenPipelines = 10000
)

var statusMap = map[string]string{
	dlStatusCompleted: "success",
	dlStatusFailed:    "failed",
	dlStatusRunning:   "running",
	dlStatusCancelled: "cancelled",
}

type pipeline struct {
	ID         uint64  `json:"id"`
	Status     string  `json:"status"`
	FinishedAt *string `json:"finishedAt"`
	BeganAt    *string `json:"beganAt"`
}

type pipelinesResponse struct {
	Pipelines []pipeline `json:"pipelines"`
	Count     int        `json:"count"`
}

type PipelinePoller struct {
	baseURL  string
	client   *http.Client
	interval time.Duration
	seen     map[uint64]string
	prevIDs  map[uint64]struct{}
	lastTS   map[string]int64

	runsTotal        *prometheus.CounterVec
	lastRunTimestamp *prometheus.GaugeVec
}

func NewPipelinePoller(baseURL string, client *http.Client, interval time.Duration, reg prometheus.Registerer) *PipelinePoller {
	p := &PipelinePoller{
		baseURL:  baseURL,
		client:   client,
		interval: interval,
		seen:     make(map[uint64]string),
		lastTS:   make(map[string]int64),
		runsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "devlake_pipeline_runs_total",
				Help: "Total number of pipeline runs by terminal status.",
			},
			[]string{"status"},
		),
		lastRunTimestamp: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "devlake_pipeline_last_run_timestamp",
				Help: "Unix timestamp of the last observed pipeline run by status.",
			},
			[]string{"status"},
		),
	}
	reg.MustRegister(p.runsTotal, p.lastRunTimestamp)
	return p
}

func (p *PipelinePoller) Run(ctx context.Context) {
	p.poll(ctx)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.poll(ctx)
		}
	}
}

func (p *PipelinePoller) poll(ctx context.Context) {
	first, ok := p.fetchPage(ctx, 1)
	if !ok {
		return
	}
	pipelines := first.Pipelines

	// Pages beyond the first are only needed when the whole previous page is
	// new, meaning pipelines may have been pushed past it since the last poll.
	// On the very first poll there is no previous state, so stay on page 1.
	if p.prevIDs != nil {
		known := make(map[uint64]struct{}, len(p.prevIDs))
		for id := range p.prevIDs {
			known[id] = struct{}{}
		}
		page := first.Pipelines
		for n := 2; n <= maxCatchUpPages+1 && needsNextPage(page, known); n++ {
			for _, pl := range page {
				known[pl.ID] = struct{}{}
			}
			next, ok := p.fetchPage(ctx, n)
			if !ok {
				break
			}
			page = next.Pipelines
			pipelines = append(pipelines, page...)
		}
	}

	newestTS := make(map[string]int64)
	for _, pl := range pipelines {
		metricStatus, ok := statusMap[pl.Status]
		if !ok {
			continue
		}

		isTerminal := pl.Status != dlStatusRunning
		if isTerminal {
			if prev, seen := p.seen[pl.ID]; seen && prev == metricStatus {
				continue
			}
			p.seen[pl.ID] = metricStatus
			p.runsTotal.WithLabelValues(metricStatus).Inc()
		}

		if ts := pipelineTimestamp(pl); ts > newestTS[metricStatus] {
			newestTS[metricStatus] = ts
		}
	}

	// The API returns pipelines newest first, so taking the last one seen
	// would report the oldest run; keep the maximum instead.
	for status, ts := range newestTS {
		if ts > p.lastTS[status] {
			p.lastTS[status] = ts
			p.lastRunTimestamp.WithLabelValues(status).Set(float64(ts))
		}
	}

	p.prevIDs = make(map[uint64]struct{}, len(pipelines))
	for _, pl := range pipelines {
		p.prevIDs[pl.ID] = struct{}{}
	}
	p.pruneSeen(pipelines)
}

// needsNextPage reports whether page is full and shares no ID with known.
// Any overlap means the next page holds nothing that hasn't been seen.
func needsNextPage(page []pipeline, known map[uint64]struct{}) bool {
	if len(page) < pipelinesPageSize {
		return false
	}
	for _, pl := range page {
		if _, ok := known[pl.ID]; ok {
			return false
		}
	}
	return true
}

func (p *PipelinePoller) pruneSeen(fetched []pipeline) {
	if len(p.seen) <= maxSeenPipelines || len(fetched) < pipelinesPageSize {
		return
	}
	minID := fetched[0].ID
	for _, pl := range fetched {
		minID = min(minID, pl.ID)
	}
	for id := range p.seen {
		if id < minID {
			delete(p.seen, id)
		}
	}
}

func (p *PipelinePoller) fetchPage(ctx context.Context, page int) (pipelinesResponse, bool) {
	var result pipelinesResponse

	url := fmt.Sprintf("%s/pipelines?pageSize=%d", p.baseURL, pipelinesPageSize)
	if page > 1 {
		url += fmt.Sprintf("&page=%d", page)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		slog.Error("failed to create pipeline request", "err", err)
		return result, false
	}

	start := time.Now()
	resp, err := p.client.Do(req)
	duration := time.Since(start).Seconds()

	if err != nil {
		slog.Warn("pipeline poll failed", "page", page, "duration_s", duration, "err", err)
		return result, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("pipeline poll returned non-200", "page", page, "status", resp.StatusCode, "duration_s", duration)
		return result, false
	}

	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&result); err != nil {
		slog.Warn("failed to decode pipeline response", "page", page, "err", err)
		return result, false
	}

	slog.Info("pipeline poll succeeded", "page", page, "status", resp.StatusCode, "pipeline_count", result.Count, "duration_s", duration)
	return result, true
}

func pipelineTimestamp(pl pipeline) int64 {
	layouts := []string{time.RFC3339, time.RFC3339Nano}
	for _, raw := range []*string{pl.FinishedAt, pl.BeganAt} {
		if raw == nil || *raw == "" {
			continue
		}
		for _, layout := range layouts {
			if t, err := time.Parse(layout, *raw); err == nil {
				return t.Unix()
			}
		}
	}
	return 0
}
