package store

import (
	"context"
	"goinmetrics/internal/model"
	"math"
	"os"
	"testing"
	"time"
)

func TestValidateRule(t *testing.T) {
	for _, r := range []model.Rule{{Metric: "cpu", Threshold: 101, Duration: 1}, {Metric: "memory", Threshold: -1, Duration: 1}, {Metric: "unknown", Duration: 10}, {Metric: "disk", Threshold: 90, Duration: 0}, {Metric: "rx", Threshold: math.NaN(), Duration: 10}} {
		if ValidateRule(r) == nil {
			t.Fatalf("invalid rule accepted: %+v", r)
		}
	}
	if err := ValidateRule(model.Rule{Metric: "rx", Threshold: 1e9, Duration: 60}); err != nil {
		t.Fatal(err)
	}
}

// TEST_DATABASE_URL must reference a disposable, dedicated test database.
func TestPostgresHistoryEventsAndRetention(t *testing.T) {
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
	if _, err = s.Pool.Exec(ctx, "TRUNCATE samples,events RESTART IDENTITY; UPDATE rules SET enabled=false"); err != nil {
		t.Fatal(err)
	}
	if err = s.SaveRule(ctx, model.Rule{Metric: "cpu", Threshold: 80, Duration: 2, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond).Add(-20 * time.Second)
	high, low := 95.0, 20.0
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
	events, err := s.Events(ctx)
	if err != nil || len(events) != 1 || events[0].State != "resolved" || !events[0].Started.Equal(now) {
		t.Fatalf("events=%+v err=%v", events, err)
	}
	points, _, err := s.History(ctx, now.Add(-time.Second), now.Add(5*time.Second))
	if err != nil || len(points) != 4 {
		t.Fatalf("points=%+v err=%v", points, err)
	}
	// A gap breaks continuous threshold timing and interrupts an active event.
	next := []model.Snapshot{{Time: now.Add(4 * time.Second), CPU: &high}, {Time: now.Add(5 * time.Second), CPU: &high}, {Time: now.Add(6 * time.Second), CPU: &high}, {Time: now.Add(10 * time.Second), CPU: &high}}
	if err = s.Write(ctx, next); err != nil {
		t.Fatal(err)
	}
	events, err = s.Events(ctx)
	if err != nil || len(events) != 2 || events[0].State != "interrupted" {
		t.Fatalf("gap events=%+v err=%v", events, err)
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
	// Reopening the store preserves saved history and interrupts old active events.
	if err = s.Write(ctx, []model.Snapshot{{Time: now.Add(11 * time.Second), CPU: &high}, {Time: now.Add(12 * time.Second), CPU: &high}, {Time: now.Add(13 * time.Second), CPU: &high}}); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Pool.Close()
	events, err = reopened.Events(ctx)
	if err != nil || events[0].State != "interrupted" {
		t.Fatalf("restart events=%+v err=%v", events, err)
	}
	points, _, err = reopened.History(ctx, now.Add(-time.Second), now.Add(15*time.Second))
	if err != nil || len(points) != 11 {
		t.Fatalf("restart points=%d err=%v", len(points), err)
	}
}
