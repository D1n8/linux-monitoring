package monitor

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"goinmetrics/internal/collector"
	"goinmetrics/internal/model"
	"goinmetrics/internal/store"
)

type Status struct {
	StorageOK    bool       `json:"storage_ok"`
	StorageError string     `json:"storage_error,omitempty"`
	LastSaved    *time.Time `json:"last_saved"`
	Dropped      uint64     `json:"dropped"`
	Missed       uint64     `json:"missed"`
	Queue        int        `json:"queue"`
	Started      time.Time  `json:"started"`
}
type Monitor struct {
	Collector   *collector.Collector
	Store       *store.Store
	mu          sync.RWMutex
	latest      model.Snapshot
	status      Status
	subscribers map[chan model.Snapshot]struct{}
	queue       chan model.Snapshot
	dropped     atomic.Uint64
	missed      atomic.Uint64
}

func New(c *collector.Collector, s *store.Store) *Monitor {
	return &Monitor{Collector: c, Store: s, subscribers: map[chan model.Snapshot]struct{}{}, queue: make(chan model.Snapshot, 60), status: Status{StorageOK: true, Started: time.Now().UTC()}}
}
func (m *Monitor) Latest() model.Snapshot { m.mu.RLock(); defer m.mu.RUnlock(); return m.latest }
func (m *Monitor) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s := m.status
	s.Dropped = m.dropped.Load()
	s.Missed = m.missed.Load()
	s.Queue = len(m.queue)
	return s
}
func (m *Monitor) Subscribe() (chan model.Snapshot, func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch := make(chan model.Snapshot, 1)
	m.subscribers[ch] = struct{}{}
	return ch, func() { m.mu.Lock(); delete(m.subscribers, ch); m.mu.Unlock() }
}
func (m *Monitor) Run(ctx context.Context) {
	writerDone := make(chan struct{})
	go func() { defer close(writerDone); m.writeLoop(ctx) }()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var previous time.Time
	for {
		now := time.Now()
		if !previous.IsZero() {
			elapsed := now.Sub(previous)
			if elapsed >= 2*time.Second {
				m.missed.Add(uint64(elapsed/time.Second) - 1)
			}
		}
		previous = now
		sample := m.Collector.Collect(now)
		m.mu.Lock()
		m.latest = sample
		for ch := range m.subscribers {
			select {
			case ch <- sample:
			default:
				select {
				case <-ch:
				default:
				}
				select {
				case ch <- sample:
				default:
				}
			}
		}
		m.mu.Unlock()
		select {
		case m.queue <- sample:
		default:
			m.dropped.Add(1)
		}
		select {
		case <-ctx.Done():
			<-writerDone
			return
		case <-ticker.C:
		}
	}
}
func (m *Monitor) storage(err error, last *time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.status.StorageOK = err == nil
	m.status.StorageError = ""
	if err != nil {
		m.status.StorageError = "Не удалось сохранить историю. Текущие метрики продолжают поступать."
		slog.Warn("storage failure", "error", err)
	}
	if last != nil {
		m.status.LastSaved = last
	}
}
func (m *Monitor) flush(batch []model.Snapshot) {
	if len(batch) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	err := m.Store.Write(ctx, batch)
	var last *time.Time
	if err == nil {
		t := batch[len(batch)-1].Time
		last = &t
	} else {
		m.dropped.Add(uint64(len(batch)))
	}
	m.storage(err, last)
}
func (m *Monitor) writeLoop(ctx context.Context) {
	batch := make([]model.Snapshot, 0, 5)
	flush := time.NewTicker(5 * time.Second)
	cleanup := time.NewTicker(time.Minute)
	defer flush.Stop()
	defer cleanup.Stop()
	for {
		select {
		case sample := <-m.queue:
			batch = append(batch, sample)
			if len(batch) >= 5 {
				m.flush(batch)
				batch = batch[:0]
			}
		case <-flush.C:
			m.flush(batch)
			batch = batch[:0]
		case <-cleanup.C:
			c, cancel := context.WithTimeout(ctx, 4*time.Second)
			err := m.Store.Cleanup(c)
			cancel()
			if err != nil {
				m.storage(err, nil)
			}
		case <-ctx.Done():
			for {
				select {
				case sample := <-m.queue:
					batch = append(batch, sample)
				default:
					m.flush(batch)
					return
				}
			}
		}
	}
}
