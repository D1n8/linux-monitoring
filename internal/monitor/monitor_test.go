package monitor

import (
	"context"
	"goinmetrics/internal/collector"
	"goinmetrics/internal/model"
	"goinmetrics/internal/store"
	"sync"
	"testing"
	"time"
)

type testRepo struct {
	mu      sync.Mutex
	enabled bool
	writes  int
	err     error
}

func (r *testRepo) Write(_ context.Context, s []model.Snapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.writes += len(s)
	return nil
}
func (r *testRepo) Cleanup(context.Context) error { return nil }
func (r *testRepo) SetEnabled(_ context.Context, b bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.err != nil {
		return r.err
	}
	r.enabled = b
	return nil
}
func (r *testRepo) History(context.Context, time.Time, time.Time) ([]store.Point, int, error) {
	return nil, 1, nil
}
func TestStopDrainsWritesAndResumeWarmsUp(t *testing.T) {
	repo := &testRepo{enabled: true}
	m := New(&collector.Collector{Proc: "/proc", Root: "/"}, repo, true)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	deadline := time.Now().Add(2 * time.Second)
	for m.Status().Collected == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond * 10)
	}
	request, cancelRequest := context.WithTimeout(ctx, 3*time.Second)
	defer cancelRequest()
	if err := m.SetEnabled(request, false); err != nil {
		t.Fatal(err)
	}
	before := m.Status()
	repo.mu.Lock()
	writes := repo.writes
	repo.mu.Unlock()
	if before.MonitoringEnabled || writes == 0 || before.Queue != 0 {
		t.Fatalf("stop did not drain: status=%+v writes=%d", before, writes)
	}
	time.Sleep(1100 * time.Millisecond)
	after := m.Status()
	repo.mu.Lock()
	afterWrites := repo.writes
	repo.mu.Unlock()
	if after.Collected != before.Collected || afterWrites != writes {
		t.Fatal("collection or persistence continued after stop was acknowledged")
	}
	if err := m.SetEnabled(request, true); err != nil {
		t.Fatal(err)
	}
	if !m.Status().MonitoringEnabled || m.Latest().CPU != nil || m.Latest().RX != nil {
		t.Fatal("resume must warm up rates instead of spanning the stopped interval")
	}
}
func TestFailedStateChangeKeepsPreviousState(t *testing.T) {
	repo := &testRepo{err: context.DeadlineExceeded}
	m := New(&collector.Collector{Proc: t.TempDir(), Root: "/"}, repo, false)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { m.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	request, cancelRequest := context.WithTimeout(ctx, time.Second)
	defer cancelRequest()
	if err := m.SetEnabled(request, true); err == nil {
		t.Fatal("storage failure hidden")
	}
	if m.Status().MonitoringEnabled || m.Status().Collected != 0 {
		t.Fatal("failed transition started collection")
	}
}
