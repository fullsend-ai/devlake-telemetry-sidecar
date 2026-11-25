package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthMethodRestriction(t *testing.T) {
	srv := New(0, false)
	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		req := httptest.NewRequest(method, "/health", nil)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s /health = %d, want 405", method, w.Code)
		}
	}
}

func TestHealthSecurityHeaders(t *testing.T) {
	srv := New(0, false)
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := w.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	var body map[string]string
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "healthy" {
		t.Errorf("status = %q, want healthy", body["status"])
	}
}

func TestPPROFDefaultOff(t *testing.T) {
	srv := New(0, false)
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("pprof off = %d, want 404", w.Code)
	}
}

func TestPPROFEnabled(t *testing.T) {
	srv := New(0, true)
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("pprof on = %d, want 200", w.Code)
	}
}

func TestServerTimeoutsAndAddress(t *testing.T) {
	srv := New(9090, false)
	if got := srv.srv.Addr; got != ":9090" {
		t.Errorf("Addr = %q, want :9090", got)
	}
	checks := []struct {
		name string
		got  time.Duration
		want time.Duration
	}{
		{"ReadHeaderTimeout", srv.srv.ReadHeaderTimeout, 10 * time.Second},
		{"ReadTimeout", srv.srv.ReadTimeout, 30 * time.Second},
		{"WriteTimeout", srv.srv.WriteTimeout, 30 * time.Second},
		{"IdleTimeout", srv.srv.IdleTimeout, 120 * time.Second},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %v, want %v", c.name, c.got, c.want)
		}
	}
}
