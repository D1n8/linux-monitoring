package collector

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCPUExcludesGuestDoubleCounting(t *testing.T) {
	c, err := parseCPU([]byte("cpu 100 20 30 800 10 4 5 6 70 10\ncpu0 1 2 3\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.total != 975 || c.idle != 810 {
		t.Fatalf("unexpected CPU counters: %+v", c)
	}
	if _, err := parseCPU([]byte("cpu bad")); err == nil {
		t.Fatal("malformed input accepted")
	}
}
func TestMemoryAvailableExcludesReclaimableCache(t *testing.T) {
	total, used, err := parseMemory([]byte("MemTotal: 1000 kB\nMemFree: 100 kB\nMemAvailable: 400 kB\nCached: 300 kB\n"))
	if err != nil || total != 1024000 || used != 614400 {
		t.Fatalf("total=%d used=%d err=%v", total, used, err)
	}
	if _, _, err := parseMemory([]byte("MemTotal: 10 kB\n")); err == nil {
		t.Fatal("missing availability must not become zero")
	}
}
func TestCollectionRatesResetsAndPartialFailures(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "net"), 0755); err != nil {
		t.Fatal(err)
	}
	write := func(name, data string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write("stat", "cpu 10 0 10 80 0 0 0 0 0 0\n")
	write("meminfo", "MemTotal: 1000 kB\nMemAvailable: 500 kB\n")
	write("net/dev", "eth0: 100 0 0 0 0 0 0 0 200 0 0 0 0 0 0 0\nlo: 999 0 0 0 0 0 0 0 999 0 0 0 0 0 0 0\n")
	c := &Collector{Proc: dir, Root: "/"}
	now := time.Now()
	first := c.Collect(now)
	if first.CPU != nil || first.RX != nil {
		t.Fatal("rates should warm up")
	}
	write("stat", "cpu 20 0 20 160 0 0 0 0 0 0\n")
	write("net/dev", "eth0: 300 0 0 0 0 0 0 0 600 0 0 0 0 0 0 0\n")
	second := c.Collect(now.Add(2 * time.Second))
	if second.CPU == nil || *second.CPU != 20 || second.RX == nil || *second.RX != 100 || *second.TX != 200 {
		t.Fatalf("bad rates: %+v", second)
	}
	write("stat", "broken")
	write("net/dev", "eth0: 1 0 0 0 0 0 0 0 2 0 0 0 0 0 0 0\n")
	third := c.Collect(now.Add(3 * time.Second))
	if third.CPU != nil || third.RX != nil {
		t.Fatal("counter reset must not become negative traffic")
	}
	if third.Memory == nil || *third.Memory != 50 || third.Errors["cpu"] == "" {
		t.Fatal("partial failure should preserve memory")
	}
}

func TestMalformedCountersAreUnavailable(t *testing.T) {
	if _, _, err := parseMemory([]byte("MemTotal: 1000 kB\nMemAvailable: invalid kB\n")); err == nil {
		t.Fatal("invalid memory must not become 100%")
	}
	if _, err := parseNetwork([]byte("eth0: invalid 0 0 0 0 0 0 0 100 0 0 0 0 0 0 0")); err == nil {
		t.Fatal("invalid network counter accepted")
	}
}

// Run explicitly on the target Linux host; reports collector cost, not PostgreSQL cost.
func BenchmarkCollect(b *testing.B) {
	c := &Collector{Proc: "/proc", Root: "/"}
	c.Collect(time.Now())
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		c.Collect(time.Now())
	}
}
