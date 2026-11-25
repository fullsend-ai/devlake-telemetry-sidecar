package collector

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func newTestProber(t *testing.T, url string) (*Prober, *prometheus.Registry) {
	t.Helper()
	reg := prometheus.NewRegistry()
	p := NewProber(url, &http.Client{Timeout: 2 * time.Second}, 30*time.Second, reg)
	return p, reg
}

func TestProbe_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p, _ := newTestProber(t, srv.URL)
	p.probeAll(context.Background())

	for _, ep := range []string{"/health", "/ping"} {
		if v := testutil.ToFloat64(p.requestsTotal.WithLabelValues(ep, "200")); v != 1 {
			t.Errorf("expected requests_total{endpoint=%q,status_code=200} = 1, got %v", ep, v)
		}
		if v := testutil.ToFloat64(p.up.WithLabelValues(ep)); v != 1 {
			t.Errorf("expected probe_up{endpoint=%q} = 1, got %v", ep, v)
		}
	}
}

func TestProbe_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p, _ := newTestProber(t, srv.URL)
	p.probe(context.Background(), "/health")

	if v := testutil.ToFloat64(p.requestsTotal.WithLabelValues("/health", "500")); v != 1 {
		t.Errorf("expected requests_total{status_code=500} = 1, got %v", v)
	}
	if v := testutil.ToFloat64(p.up.WithLabelValues("/health")); v != 0 {
		t.Errorf("expected probe_up = 0, got %v", v)
	}
}

func TestProbe_ConnectionError(t *testing.T) {
	p, _ := newTestProber(t, "http://127.0.0.1:1")
	p.probe(context.Background(), "/health")

	if v := testutil.ToFloat64(p.requestsTotal.WithLabelValues("/health", "0")); v != 1 {
		t.Errorf("expected requests_total{status_code=0} = 1, got %v", v)
	}
	if v := testutil.ToFloat64(p.up.WithLabelValues("/health")); v != 0 {
		t.Errorf("expected probe_up = 0, got %v", v)
	}
}
