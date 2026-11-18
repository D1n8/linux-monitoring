package store

import (
	"context"
	"goinmetrics/internal/model"
	"os"
	"testing"
	"time"
)

// TEST_DATABASE_URL must reference a disposable, dedicated test database.
func TestPostgresHistorySettingsAndRetention(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set TEST_DATABASE_URL to run PostgreSQL integration")
	}
	ctx := context.Background()
	s, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Pool.Close()
	if _, err = s.Pool.Exec(ctx, "TRUNCATE samples; UPDATE service_settings SET monitoring_enabled=true"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second).Add(-20 * time.Second)
	high, low := 95., 20.
	samples := []model.Snapshot{}
	for i := 0; i < 4; i++ {
		v := high
		if i == 3 {
			v = low
		}
		samples = append(samples, model.Snapshot{Time: now.Add(time.Duration(i) * time.Second), CPU: &v})
	}
	if err = s.Write(ctx, samples); err != nil {
		t.Fatal(err)
	}
	if err = s.Write(ctx, samples); err != nil {
		t.Fatal(err)
	} // Retrying a timestamp does not duplicate it.
	points, _, err := s.History(ctx, now.Add(-time.Second), now.Add(5*time.Second))
	if err != nil || len(points) != 4 {
		t.Fatalf("points=%+v err=%v", points, err)
	}
	// Adjacent comparison intervals must not count a boundary sample twice.
	left, _, err := s.History(ctx, now, now.Add(2*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	right, _, err := s.History(ctx, now.Add(2*time.Second), now.Add(4*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 2 || len(right) != 2 {
		t.Fatalf("adjacent ranges overlap: left=%d right=%d", len(left), len(right))
	}
	if err = s.SetEnabled(ctx, false); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Pool.Close()
	enabled, err := reopened.Enabled(ctx)
	if err != nil || enabled {
		t.Fatalf("disabled state lost: enabled=%v err=%v", enabled, err)
	}
	points, _, err = reopened.History(ctx, now.Add(-time.Second), now.Add(5*time.Second))
	if err != nil || len(points) != 4 {
		t.Fatalf("restart points=%d err=%v", len(points), err)
	}
	if err = s.Write(ctx, []model.Snapshot{{Time: now.Add(-25 * time.Hour), CPU: &low}}); err != nil {
		t.Fatal(err)
	}
	if err = s.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = s.Pool.QueryRow(ctx, "SELECT count(*) FROM samples WHERE time < now()-interval '24 hours'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("retention count=%d err=%v", count, err)
	}
	// Downsampling retains the peak. A missing metric does not become zero.
	if _, err = s.Pool.Exec(ctx, "TRUNCATE samples"); err != nil {
		t.Fatal(err)
	}
	bucket := now.Truncate(144 * time.Second)
	if err = s.Write(ctx, []model.Snapshot{{Time: bucket, CPU: &low}, {Time: bucket.Add(time.Second), CPU: &high}}); err != nil {
		t.Fatal(err)
	}
	points, _, err = s.History(ctx, bucket.Add(-12*time.Hour), bucket.Add(12*time.Hour))
	if err != nil || len(points) != 1 || points[0].CPU == nil || *points[0].CPUMax != 95 || *points[0].CPUMin != 20 {
		t.Fatalf("aggregates=%+v err=%v", points, err)
	}
	if err = s.Write(ctx, []model.Snapshot{{Time: bucket.Add(2 * time.Second)}}); err != nil {
		t.Fatal(err)
	}
	points, _, err = s.History(ctx, bucket.Add(-12*time.Hour), bucket.Add(12*time.Hour))
	if err != nil || points[0].CPU != nil {
		t.Fatalf("missing source must break bucket: %+v err=%v", points, err)
	}
}
