package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

// fakeDevLake serves /pipelines newest first (ID descending) with real
// page/pageSize semantics and records every request URL.
type fakeDevLake struct {
	mu        sync.Mutex
	pipelines []pipeline
	requests  []string
	ignorePg  bool
}

func (f *fakeDevLake) set(pls []pipeline) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pipelines = pls
}

func (f *fakeDevLake) reqs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...)
}

func (f *fakeDevLake) resetReqs() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = nil
}

func (f *fakeDevLake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r.URL.RequestURI())

	size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 || f.ignorePg {
		page = 1
	}
	start := min((page-1)*size, len(f.pipelines))
	end := min(start+size, len(f.pipelines))

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(pipelinesResponse{Pipelines: f.pipelines[start:end], Count: len(f.pipelines)})
}

// completedRange returns completed pipelines with IDs hi..lo (descending),
// each finishing one minute after the previous ID.
func completedRange(hi, lo uint64) []pipeline {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	var out []pipeline
	for id := hi; id >= lo; id-- {
		ts := base.Add(time.Duration(id) * time.Minute).Format(time.RFC3339)
		out = append(out, pipeline{ID: id, Status: dlStatusCompleted, FinishedAt: strPtr(ts)})
		if id == 0 {
			break
		}
	}
	return out
}

func tsForID(id uint64) float64 {
	return float64(time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(id) * time.Minute).Unix())
}

func TestPoll_FirstRequestURLUnchanged(t *testing.T) {
	f := &fakeDevLake{}
	srv := httptest.NewServer(f)
	defer srv.Close()

	p, _ := newTestPoller(t, srv.URL)
	p.poll(context.Background())

	got := f.reqs()
	if len(got) != 1 || got[0] != "/pipelines?pageSize=100" {
		t.Fatalf("expected single request to /pipelines?pageSize=100, got %v", got)
	}
}

func TestPoll_LastRunTimestampIsNewestNotLastInList(t *testing.T) {
	srv := servePipelines(t, []pipeline{
		{ID: 3, Status: dlStatusCompleted, FinishedAt: strPtr("2025-01-15T12:00:00Z")},
		{ID: 2, Status: dlStatusCompleted, FinishedAt: strPtr("2025-01-15T11:00:00Z")},
		{ID: 1, Status: dlStatusCompleted, FinishedAt: strPtr("2025-01-15T10:00:00Z")},
	})
	defer srv.Close()

	p, _ := newTestPoller(t, srv.URL)
	p.poll(context.Background())

	want := float64(time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC).Unix())
	if v := testutil.ToFloat64(p.lastRunTimestamp.WithLabelValues("success")); v != want {
		t.Errorf("expected newest timestamp %v, got %v", want, v)
	}
}

func TestPoll_LastRunTimestampNeverGoesBackwards(t *testing.T) {
	f := &fakeDevLake{}
	f.set([]pipeline{{ID: 2, Status: dlStatusFailed, FinishedAt: strPtr("2025-01-15T12:00:00Z")}})
	srv := httptest.NewServer(f)
	defer srv.Close()

	p, _ := newTestPoller(t, srv.URL)
	p.poll(context.Background())

	// Newest pipeline deleted; only an older one remains.
	f.set([]pipeline{{ID: 1, Status: dlStatusFailed, FinishedAt: strPtr("2025-01-15T10:00:00Z")}})
	p.poll(context.Background())

	want := float64(time.Date(2025, 1, 15, 12, 0, 0, 0, time.UTC).Unix())
	if v := testutil.ToFloat64(p.lastRunTimestamp.WithLabelValues("failed")); v != want {
		t.Errorf("expected timestamp to stay at %v, got %v", want, v)
	}
}

func TestPoll_RunningTimestampUsesBeganAt(t *testing.T) {
	srv := servePipelines(t, []pipeline{
		{ID: 1, Status: dlStatusRunning, BeganAt: strPtr("2025-01-15T10:00:00Z")},
	})
	defer srv.Close()

	p, _ := newTestPoller(t, srv.URL)
	p.poll(context.Background())

	want := float64(time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC).Unix())
	if v := testutil.ToFloat64(p.lastRunTimestamp.WithLabelValues("running")); v != want {
		t.Errorf("expected running timestamp %v, got %v", want, v)
	}
}

func TestPoll_StatusChangeIsCountedAgain(t *testing.T) {
	f := &fakeDevLake{}
	f.set([]pipeline{{ID: 1, Status: dlStatusFailed, FinishedAt: strPtr("2025-01-15T10:00:00Z")}})
	srv := httptest.NewServer(f)
	defer srv.Close()

	p, _ := newTestPoller(t, srv.URL)
	p.poll(context.Background())
	f.set([]pipeline{{ID: 1, Status: dlStatusCompleted, FinishedAt: strPtr("2025-01-15T11:00:00Z")}})
	p.poll(context.Background())

	if v := testutil.ToFloat64(p.runsTotal.WithLabelValues("failed")); v != 1 {
		t.Errorf("expected failed = 1, got %v", v)
	}
	if v := testutil.ToFloat64(p.runsTotal.WithLabelValues("success")); v != 1 {
		t.Errorf("expected success = 1, got %v", v)
	}
}

func TestPoll_FirstPollOnlyReadsFirstPage(t *testing.T) {
	f := &fakeDevLake{}
	f.set(completedRange(250, 1))
	srv := httptest.NewServer(f)
	defer srv.Close()

	p, _ := newTestPoller(t, srv.URL)
	p.poll(context.Background())

	if n := len(f.reqs()); n != 1 {
		t.Fatalf("expected 1 request on first poll, got %d: %v", n, f.reqs())
	}
	if v := testutil.ToFloat64(p.runsTotal.WithLabelValues("success")); v != 100 {
		t.Errorf("expected 100 counted on first poll, got %v", v)
	}
}

func TestPoll_NoExtraPagesWhenFirstPageOverlaps(t *testing.T) {
	f := &fakeDevLake{}
	f.set(completedRange(150, 1))
	srv := httptest.NewServer(f)
	defer srv.Close()

	p, _ := newTestPoller(t, srv.URL)
	p.poll(context.Background())
	f.resetReqs()

	// 50 new pipelines: page 1 still contains IDs from the previous poll.
	f.set(completedRange(200, 1))
	p.poll(context.Background())

	if got := f.reqs(); len(got) != 1 {
		t.Fatalf("expected a single request, got %v", got)
	}
	if v := testutil.ToFloat64(p.runsTotal.WithLabelValues("success")); v != 150 {
		t.Errorf("expected 100 + 50 = 150, got %v", v)
	}
}

func TestPoll_CatchesUpWhenMoreThanOnePageChanged(t *testing.T) {
	f := &fakeDevLake{}
	f.set(completedRange(100, 1))
	srv := httptest.NewServer(f)
	defer srv.Close()

	p, _ := newTestPoller(t, srv.URL)
	p.poll(context.Background())
	f.resetReqs()

	// 250 new pipelines since the last poll: needs pages 1-3, page 4 overlaps.
	f.set(completedRange(350, 1))
	p.poll(context.Background())

	got := f.reqs()
	want := []string{
		"/pipelines?pageSize=100",
		"/pipelines?pageSize=100&page=2",
		"/pipelines?pageSize=100&page=3",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("expected requests %v, got %v", want, got)
	}
	if v := testutil.ToFloat64(p.runsTotal.WithLabelValues("success")); v != 350 {
		t.Errorf("expected every pipeline counted exactly once (350), got %v", v)
	}
	if v := testutil.ToFloat64(p.lastRunTimestamp.WithLabelValues("success")); v != tsForID(350) {
		t.Errorf("expected newest timestamp %v, got %v", tsForID(350), v)
	}
}

func TestPoll_CatchUpIsBounded(t *testing.T) {
	f := &fakeDevLake{}
	f.set(completedRange(100, 1))
	srv := httptest.NewServer(f)
	defer srv.Close()

	p, _ := newTestPoller(t, srv.URL)
	p.poll(context.Background())
	f.resetReqs()

	f.set(completedRange(5000, 1))
	p.poll(context.Background())

	if n := len(f.reqs()); n != 1+maxCatchUpPages {
		t.Fatalf("expected %d requests, got %d", 1+maxCatchUpPages, n)
	}
}

func TestPoll_CatchUpStopsWhenServerIgnoresPageParam(t *testing.T) {
	f := &fakeDevLake{ignorePg: true}
	f.set(completedRange(100, 1))
	srv := httptest.NewServer(f)
	defer srv.Close()

	p, _ := newTestPoller(t, srv.URL)
	p.poll(context.Background())
	f.resetReqs()

	f.set(completedRange(400, 1))
	p.poll(context.Background())

	if n := len(f.reqs()); n != 2 {
		t.Fatalf("expected catch-up to stop after a repeated page (2 requests), got %d", n)
	}
	if v := testutil.ToFloat64(p.runsTotal.WithLabelValues("success")); v != 200 {
		t.Errorf("expected no double counting (200), got %v", v)
	}
}

func TestPoll_CatchUpPageFailureKeepsFirstPage(t *testing.T) {
	var mu sync.Mutex
	pls := completedRange(100, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.URL.Query().Get("page") != "" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(pipelinesResponse{Pipelines: pls[:100], Count: len(pls)})
	}))
	defer srv.Close()

	p, _ := newTestPoller(t, srv.URL)
	p.poll(context.Background())
	mu.Lock()
	pls = completedRange(300, 1)
	mu.Unlock()
	p.poll(context.Background())

	if v := testutil.ToFloat64(p.runsTotal.WithLabelValues("success")); v != 200 {
		t.Errorf("expected first page still processed (200), got %v", v)
	}
}

func TestPoll_FailedPollKeepsState(t *testing.T) {
	var mu sync.Mutex
	fail := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if fail {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_ = json.NewEncoder(w).Encode(pipelinesResponse{Pipelines: completedRange(3, 1), Count: 3})
	}))
	defer srv.Close()

	p, _ := newTestPoller(t, srv.URL)
	p.poll(context.Background())
	mu.Lock()
	fail = true
	mu.Unlock()
	p.poll(context.Background())
	mu.Lock()
	fail = false
	mu.Unlock()
	p.poll(context.Background())

	if v := testutil.ToFloat64(p.runsTotal.WithLabelValues("success")); v != 3 {
		t.Errorf("expected 3 after fail/recover cycle, got %v", v)
	}
}

func TestPoll_SeenIsPrunedOnlyAboveLimit(t *testing.T) {
	f := &fakeDevLake{}
	srv := httptest.NewServer(f)
	defer srv.Close()

	p, _ := newTestPoller(t, srv.URL)
	for id := uint64(1); id <= maxSeenPipelines; id++ {
		p.seen[id] = "success"
	}
	f.set(completedRange(maxSeenPipelines+100, maxSeenPipelines-99))
	p.poll(context.Background())

	if len(p.seen) > 200 {
		t.Fatalf("expected seen to be pruned to the current window, got %d entries", len(p.seen))
	}
	for _, pl := range completedRange(maxSeenPipelines+100, maxSeenPipelines-99)[:100] {
		if _, ok := p.seen[pl.ID]; !ok {
			t.Fatalf("pipeline %d in current window was pruned", pl.ID)
		}
	}

	// Re-polling the same window must not recount anything.
	before := testutil.ToFloat64(p.runsTotal.WithLabelValues("success"))
	p.poll(context.Background())
	if after := testutil.ToFloat64(p.runsTotal.WithLabelValues("success")); after != before {
		t.Errorf("expected no recount after pruning, %v -> %v", before, after)
	}
}

func TestPoll_SeenNotPrunedBelowLimit(t *testing.T) {
	f := &fakeDevLake{}
	srv := httptest.NewServer(f)
	defer srv.Close()

	p, _ := newTestPoller(t, srv.URL)
	f.set(completedRange(100, 1))
	p.poll(context.Background())
	f.set(completedRange(200, 101))
	p.poll(context.Background())

	if len(p.seen) != 200 {
		t.Fatalf("expected all 200 IDs retained below the limit, got %d", len(p.seen))
	}
}

func TestPoll_OversizedResponseIsRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"pipelines":[{"id":1,"status":"TASK_COMPLETED","finishedAt":"`)
		chunk := strings.Repeat("x", 1<<20)
		for i := 0; i < (maxResponseBytes>>20)+2; i++ {
			fmt.Fprint(w, chunk)
		}
		fmt.Fprint(w, `"}],"count":1}`)
	}))
	defer srv.Close()

	p, reg := newTestPoller(t, srv.URL)
	p.poll(context.Background())

	if n, _ := testutil.GatherAndCount(reg, "devlake_pipeline_runs_total"); n != 0 {
		t.Errorf("expected oversized response to be discarded, got %d series", n)
	}
}
