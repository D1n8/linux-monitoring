package store

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"goinmetrics/internal/model"
)

//go:embed schema.sql
var schema string

type ruleState struct {
	Since    time.Time
	Last     time.Time
	EventID  int64
	Revision int64
}
type Store struct {
	Pool   *pgxpool.Pool
	states map[string]ruleState
}

func Open(ctx context.Context, url string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("параметры БД: %w", err)
	}
	cfg.MaxConns = 4
	cfg.MinConns = 0
	cfg.ConnConfig.ConnectTimeout = 3 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	s := &Store{Pool: pool, states: map[string]ruleState{}}
	if err = pool.Ping(ctx); err == nil {
		_, err = pool.Exec(ctx, schema)
	}
	if err == nil {
		_, err = pool.Exec(ctx, "UPDATE events SET state='interrupted',ended=now() WHERE state='active'")
	}
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("подготовка PostgreSQL: %w", err)
	}
	return s, nil
}
func (s *Store) Rules(ctx context.Context) ([]model.Rule, error) {
	rows, err := s.Pool.Query(ctx, "SELECT metric,threshold,duration,enabled FROM rules ORDER BY metric")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []model.Rule{}
	for rows.Next() {
		var r model.Rule
		if err := rows.Scan(&r.Metric, &r.Threshold, &r.Duration, &r.Enabled); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	return result, rows.Err()
}
func ValidateRule(r model.Rule) error {
	if r.Metric != "cpu" && r.Metric != "memory" && r.Metric != "disk" && r.Metric != "rx" && r.Metric != "tx" {
		return fmt.Errorf("неизвестный показатель")
	}
	if math.IsNaN(r.Threshold) || math.IsInf(r.Threshold, 0) || r.Threshold < 0 || ((r.Metric == "cpu" || r.Metric == "memory" || r.Metric == "disk") && r.Threshold > 100) || r.Threshold > 1e15 {
		return fmt.Errorf("порог вне допустимого диапазона")
	}
	if r.Duration < 1 || r.Duration > 3600 {
		return fmt.Errorf("длительность должна быть от 1 до 3600 секунд")
	}
	return nil
}
func (s *Store) SaveRule(ctx context.Context, r model.Rule) error {
	if err := ValidateRule(r); err != nil {
		return err
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO rules(metric,threshold,duration,enabled) VALUES($1,$2,$3,$4)
 ON CONFLICT(metric) DO UPDATE SET threshold=$2,duration=$3,enabled=$4,revision=rules.revision+1`, r.Metric, r.Threshold, r.Duration, r.Enabled)
	return err
}

// Write commits a bounded group of samples and alert transitions atomically.
// A failed transaction never advances the in-memory threshold timer.
func (s *Store) Write(ctx context.Context, samples []model.Snapshot) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	next := make(map[string]ruleState, len(s.states))
	for k, v := range s.states {
		next[k] = v
	}
	type versioned struct {
		model.Rule
		revision int64
	}
	rules := []versioned{}
	rows, err := tx.Query(ctx, "SELECT metric,threshold,duration,enabled,revision FROM rules ORDER BY metric FOR SHARE")
	if err != nil {
		return err
	}
	for rows.Next() {
		var r versioned
		if err = rows.Scan(&r.Metric, &r.Threshold, &r.Duration, &r.Enabled, &r.revision); err != nil {
			rows.Close()
			return err
		}
		rules = append(rules, r)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	batch := &pgx.Batch{}
	for _, sample := range samples {
		data, e := json.Marshal(sample)
		if e != nil {
			return e
		}
		batch.Queue("INSERT INTO samples(time,data) VALUES($1,$2) ON CONFLICT DO NOTHING", sample.Time, data)
	}
	if err = tx.SendBatch(ctx, batch).Close(); err != nil {
		return err
	}
	for _, sample := range samples {
		for _, r := range rules {
			state := next[r.Metric]
			value := sample.Value(r.Metric)
			interrupted := state.Revision != r.revision || (!state.Last.IsZero() && sample.Time.Sub(state.Last) > 2500*time.Millisecond) || value == nil
			if interrupted {
				if state.EventID != 0 {
					if _, err = tx.Exec(ctx, "UPDATE events SET state='interrupted',ended=$2 WHERE id=$1", state.EventID, sample.Time); err != nil {
						return err
					}
				}
				state = ruleState{Revision: r.revision}
			}
			if !r.Enabled || value == nil || *value <= r.Threshold {
				if state.EventID != 0 {
					if _, err = tx.Exec(ctx, "UPDATE events SET state='resolved',ended=$2 WHERE id=$1", state.EventID, sample.Time); err != nil {
						return err
					}
				}
				state.Since = time.Time{}
				state.EventID = 0
			} else {
				if state.Since.IsZero() {
					state.Since = sample.Time
				}
				if state.EventID == 0 && sample.Time.Sub(state.Since) >= time.Duration(r.Duration)*time.Second {
					err = tx.QueryRow(ctx, "INSERT INTO events(metric,threshold,started,state) VALUES($1,$2,$3,'active') RETURNING id", r.Metric, r.Threshold, state.Since).Scan(&state.EventID)
					if err != nil {
						return err
					}
				}
			}
			state.Last = sample.Time
			next[r.Metric] = state
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return err
	}
	s.states = next
	return nil
}
func (s *Store) Cleanup(ctx context.Context) error {
	// Limit each sweep so recovery after a long stop does not create one large delete.
	_, err := s.Pool.Exec(ctx, `DELETE FROM samples WHERE time IN (SELECT time FROM samples WHERE time < now()-interval '24 hours' ORDER BY time LIMIT 10000)`)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, "DELETE FROM events WHERE ended < now()-interval '24 hours'")
	return err
}

type Point struct {
	Time      time.Time `json:"time"`
	CPU       *float64  `json:"cpu"`
	CPUMin    *float64  `json:"cpu_min"`
	CPUMax    *float64  `json:"cpu_max"`
	Memory    *float64  `json:"memory"`
	MemoryMin *float64  `json:"memory_min"`
	MemoryMax *float64  `json:"memory_max"`
	Disk      *float64  `json:"disk"`
	DiskMin   *float64  `json:"disk_min"`
	DiskMax   *float64  `json:"disk_max"`
	RX        *float64  `json:"rx"`
	RXMin     *float64  `json:"rx_min"`
	RXMax     *float64  `json:"rx_max"`
	TX        *float64  `json:"tx"`
	TXMin     *float64  `json:"tx_min"`
	TXMax     *float64  `json:"tx_max"`
	Count     int       `json:"count"`
}

func (s *Store) History(ctx context.Context, from, to time.Time) ([]Point, int, error) {
	step := int(math.Ceil(to.Sub(from).Seconds() / 600))
	if step < 1 {
		step = 1
	}
	query := `SELECT to_timestamp(floor(extract(epoch from time)/$3)*$3),`
	for _, metric := range []string{"cpu", "memory", "disk", "rx", "tx"} {
		query += fmt.Sprintf("CASE WHEN count(data->>'%s')=count(*) THEN avg((data->>'%s')::float8) END,min((data->>'%s')::float8),max((data->>'%s')::float8),", metric, metric, metric, metric)
	}
	query += `count(*),extract(epoch from max(time)-min(time))::float8 FROM samples WHERE time >= $1 AND time <= $2 GROUP BY 1 ORDER BY 1`
	rows, err := s.Pool.Query(ctx, query, from, to, step)
	if err != nil {
		return nil, step, err
	}
	defer rows.Close()
	points := []Point{}
	for rows.Next() {
		var p Point
		var span float64
		if err := rows.Scan(&p.Time, &p.CPU, &p.CPUMin, &p.CPUMax, &p.Memory, &p.MemoryMin, &p.MemoryMax, &p.Disk, &p.DiskMin, &p.DiskMax, &p.RX, &p.RXMin, &p.RXMax, &p.TX, &p.TXMin, &p.TXMax, &p.Count, &span); err != nil {
			return nil, step, err
		}
		if span > float64(p.Count)+0.5 {
			p.CPU = nil
			p.Memory = nil
			p.Disk = nil
			p.RX = nil
			p.TX = nil
		}
		points = append(points, p)
	}
	return points, step, rows.Err()
}
func (s *Store) Events(ctx context.Context) ([]model.Event, error) {
	rows, err := s.Pool.Query(ctx, "SELECT id,metric,threshold,started,ended,state FROM events ORDER BY started DESC LIMIT 100")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []model.Event{}
	for rows.Next() {
		var e model.Event
		if err = rows.Scan(&e.ID, &e.Metric, &e.Threshold, &e.Started, &e.Ended, &e.State); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}
