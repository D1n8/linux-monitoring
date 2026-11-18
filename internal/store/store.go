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

type Store struct{ Pool *pgxpool.Pool }

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
	s := &Store{Pool: pool}
	if err = pool.Ping(ctx); err == nil {
		_, err = pool.Exec(ctx, schema)
	}
	if err != nil {
		pool.Close()
		return nil, fmt.Errorf("подготовка PostgreSQL: %w", err)
	}
	return s, nil
}

// Enabled is persisted so an intentional stop survives a process or Docker restart.
func (s *Store) Enabled(ctx context.Context) (bool, error) {
	var enabled bool
	err := s.Pool.QueryRow(ctx, "SELECT monitoring_enabled FROM service_settings WHERE id=1").Scan(&enabled)
	return enabled, err
}
func (s *Store) SetEnabled(ctx context.Context, enabled bool) error {
	_, err := s.Pool.Exec(ctx, "UPDATE service_settings SET monitoring_enabled=$1 WHERE id=1", enabled)
	return err
}
func (s *Store) Write(ctx context.Context, samples []model.Snapshot) error {
	if len(samples) == 0 {
		return nil
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
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
	return tx.Commit(ctx)
}
func (s *Store) Cleanup(ctx context.Context) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM samples WHERE time IN (SELECT time FROM samples WHERE time < now()-interval '24 hours' ORDER BY time LIMIT 10000)`)
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
	query += `count(*),extract(epoch from max(time)-min(time))::float8 FROM samples WHERE time >= $1 AND time < $2 GROUP BY 1 ORDER BY 1`
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
