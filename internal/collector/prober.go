package collector

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type Prober struct {
	baseURL   string
	client    *http.Client
	interval  time.Duration
	endpoints []string

	requestsTotal   *prometheus.CounterVec
	durationSeconds *prometheus.HistogramVec
	up              *prometheus.GaugeVec
}

func NewProber(baseURL string, client *http.Client, interval time.Duration, reg prometheus.Registerer) *Prober {
	p := &Prober{
		baseURL:   baseURL,
		client:    client,
		interval:  interval,
		endpoints: []string{"/health", "/ping"},
		requestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "devlake_probe_requests_total",
				Help: "Total number of probe requests against the DevLake API.",
			},
			[]string{"endpoint", "status_code"},
		),
		durationSeconds: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "devlake_probe_duration_seconds",
				Help:    "Duration of probe requests against the DevLake API.",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"endpoint"},
		),
		up: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "devlake_probe_up",
				Help: "Whether the DevLake API endpoint is up (1) or down (0).",
			},
			[]string{"endpoint"},
		),
	}
	reg.MustRegister(p.requestsTotal, p.durationSeconds, p.up)
	return p
}

func (p *Prober) Run(ctx context.Context) {
	p.probeAll(ctx)
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.probeAll(ctx)
		}
	}
}

func (p *Prober) probeAll(ctx context.Context) {
	for _, endpoint := range p.endpoints {
		p.probe(ctx, endpoint)
	}
}

func (p *Prober) probe(ctx context.Context, endpoint string) {
	url := p.baseURL + endpoint
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		slog.Warn("failed to create probe request", "endpoint", endpoint, "err", err)
		p.requestsTotal.WithLabelValues(endpoint, "0").Inc()
		p.up.WithLabelValues(endpoint).Set(0)
		return
	}

	start := time.Now()
	resp, err := p.client.Do(req)
	duration := time.Since(start).Seconds()

	p.durationSeconds.WithLabelValues(endpoint).Observe(duration)

	if err != nil {
		slog.Warn("probe request failed", "endpoint", endpoint, "duration_s", duration, "err", err)
		p.requestsTotal.WithLabelValues(endpoint, "0").Inc()
		p.up.WithLabelValues(endpoint).Set(0)
		return
	}
	_ = resp.Body.Close()

	statusCode := strconv.Itoa(resp.StatusCode)
	p.requestsTotal.WithLabelValues(endpoint, statusCode).Inc()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		slog.Info("probe request succeeded", "endpoint", endpoint, "status", resp.StatusCode, "duration_s", duration)
		p.up.WithLabelValues(endpoint).Set(1)
	} else {
		slog.Warn("probe request returned non-2xx", "endpoint", endpoint, "status", resp.StatusCode, "duration_s", duration)
		p.up.WithLabelValues(endpoint).Set(0)
	}
}
