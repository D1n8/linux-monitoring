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

type Repository interface {
	Write(context.Context, []model.Snapshot) error
	Cleanup(context.Context) error
	SetEnabled(context.Context, bool) error
	History(context.Context, time.Time, time.Time) ([]store.Point, int, error)
}
type Status struct {
	Viewers           int        `json:"viewers"`
	Collected         uint64     `json:"collected"`
	MonitoringEnabled bool       `json:"monitoring_enabled"`
	StorageOK         bool       `json:"storage_ok"`
	StorageError      string     `json:"storage_error,omitempty"`
	LastSaved         *time.Time `json:"last_saved"`
	Dropped           uint64     `json:"dropped"`
	Missed            uint64     `json:"missed"`
	Queue             int        `json:"queue"`
	Started           time.Time  `json:"started"`
}
type Update struct {
	Sample *model.Snapshot
	Status Status
}
type control struct {
	ctx     context.Context
	enabled bool
	done    chan error
}
type writeJob struct {
	sample  *model.Snapshot
	barrier chan struct{}
}
type Monitor struct {
	host        model.Host
	hostTime    time.Time
	collected   atomic.Uint64
	Collector   *collector.Collector
	Store       Repository
	mu          sync.RWMutex
	latest      model.Snapshot
	status      Status
	subscribers map[chan Update]struct{}
	queue       chan writeJob
	controls    chan control
	dropped     atomic.Uint64
	missed      atomic.Uint64
}

func New(c *collector.Collector, s Repository, enabled bool) *Monitor {
	return &Monitor{host: c.Host(), hostTime: time.Now(), Collector: c, Store: s, subscribers: map[chan Update]struct{}{}, queue: make(chan writeJob, 60), controls: make(chan control), status: Status{MonitoringEnabled: enabled, StorageOK: true, Started: time.Now().UTC()}}
}
func (m *Monitor) Latest() model.Snapshot { m.mu.RLock(); defer m.mu.RUnlock(); return m.latest }
func (m *Monitor) statusLocked() Status {
	s := m.status
	s.Viewers = len(m.subscribers)
	s.Collected = m.collected.Load()
	s.Dropped = m.dropped.Load()
	s.Missed = m.missed.Load()
	s.Queue = len(m.queue)
	return s
}
func (m *Monitor) Status() Status { m.mu.RLock(); defer m.mu.RUnlock(); return m.statusLocked() }
func (m *Monitor) Subscribe() (chan Update, func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch := make(chan Update, 1)
	m.subscribers[ch] = struct{}{}
	return ch, func() { m.mu.Lock(); delete(m.subscribers, ch); m.mu.Unlock() }
}
func (m *Monitor) publishLocked(sample *model.Snapshot) {
	update := Update{Sample: sample, Status: m.statusLocked()}
	for ch := range m.subscribers {
		select {
		case ch <- update:
		default:
			select {
			case <-ch:
			default:
			}
			select {
			case ch <- update:
			default:
			}
		}
	}
}
func (m *Monitor) SetEnabled(ctx context.Context, enabled bool) error {
	request := control{ctx: ctx, enabled: enabled, done: make(chan error, 1)}
	select {
	case m.controls <- request:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-request.done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (m *Monitor) setEnabled(request control) error {
	if err := request.ctx.Err(); err != nil {
		return err
	}
	if request.enabled == m.Status().MonitoringEnabled {
		return nil
	}
	// The sampler is suspended in this function. A FIFO barrier drains every
	// previously queued sample before a stop can be acknowledged.
	if !request.enabled {
		barrier := make(chan struct{})
		select {
		case m.queue <- writeJob{barrier: barrier}:
		case <-request.ctx.Done():
			return request.ctx.Err()
		}
		select {
		case <-barrier:
		case <-request.ctx.Done():
			return request.ctx.Err()
		}
	}
	if err := m.Store.SetEnabled(request.ctx, request.enabled); err != nil {
		return err
	}
	m.Collector.Reset()
	m.mu.Lock()
	m.status.MonitoringEnabled = request.enabled
	m.publishLocked(nil)
	m.mu.Unlock()
	return nil
}
func (m *Monitor) Run(ctx context.Context) {
	writerDone := make(chan struct{})
	go func() { defer close(writerDone); m.writeLoop(ctx) }()
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var previous time.Time
	collect := func() {
		now := time.Now()
		if !previous.IsZero() {
			elapsed := now.Sub(previous)
			if elapsed >= 2*time.Second {
				m.missed.Add(uint64(elapsed/time.Second) - 1)
			}
		}
		previous = now
		sample := m.Collector.Collect(now)
		m.collected.Add(1)
		m.mu.Lock()
		m.latest = sample
		m.publishLocked(&sample)
		m.mu.Unlock()
		select {
		case m.queue <- writeJob{sample: &sample}:
		default:
			m.dropped.Add(1)
		}
	}
	if m.Status().MonitoringEnabled {
		collect()
	}
	for {
		select {
		case <-ctx.Done():
			<-writerDone
			return
		case request := <-m.controls:
			wasEnabled := m.Status().MonitoringEnabled
			err := m.setEnabled(request)
			if err == nil && wasEnabled != request.enabled {
				previous = time.Time{}
				ticker.Reset(time.Second)
				if request.enabled {
					collect()
				}
			}
			request.done <- err
		case <-ticker.C:
			if m.Status().MonitoringEnabled {
				collect()
			}
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
	m.publishLocked(nil)
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
	handle := func(job writeJob) {
		if job.sample != nil {
			batch = append(batch, *job.sample)
		}
		if len(batch) >= 5 || job.barrier != nil {
			m.flush(batch)
			batch = batch[:0]
		}
		if job.barrier != nil {
			close(job.barrier)
		}
	}
	for {
		select {
		case job := <-m.queue:
			handle(job)
		case <-flush.C:
			m.flush(batch)
			batch = batch[:0]
		case <-cleanup.C:
			if !m.Status().MonitoringEnabled {
				continue
			}
			c, cancel := context.WithTimeout(ctx, 4*time.Second)
			err := m.Store.Cleanup(c)
			cancel()
			if err != nil {
				m.storage(err, nil)
			}
		case <-ctx.Done():
			for {
				select {
				case job := <-m.queue:
					handle(job)
				default:
					m.flush(batch)
					return
				}
			}
		}
	}
}

// Host metadata is immutable; uptime advances without rereading cpuinfo per viewer.
func (m *Monitor) Host() model.Host {
	h := m.host
	h.Uptime += time.Since(m.hostTime).Seconds()
	return h
}
