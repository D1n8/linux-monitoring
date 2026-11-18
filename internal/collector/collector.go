// Package collector reads Linux counters directly. No subprocesses or per-viewer collectors.
package collector

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"goinmetrics/internal/model"
)

type cpuCounter struct{ total, idle uint64 }
type netCounter struct{ rx, tx uint64 }
type mount struct{ device, path string }
type Collector struct {
	Proc, Root   string
	previousCPU  *cpuCounter
	previousNet  map[string]netCounter
	previousTime time.Time
	mounts       []mount
	mountRefresh time.Time
}

func number(s string) uint64 { n, _ := strconv.ParseUint(s, 10, 64); return n }
func ptr(f float64) *float64 { return &f }
func (c *Collector) read(name string) ([]byte, error) {
	return os.ReadFile(filepath.Join(c.Proc, name))
}
func parseCPU(data []byte) (cpuCounter, error) {
	fields := strings.Fields(strings.SplitN(string(data), "\n", 2)[0])
	if len(fields) < 9 || fields[0] != "cpu" {
		return cpuCounter{}, fmt.Errorf("неверный формат /proc/stat")
	}
	var out cpuCounter
	// guest and guest_nice are already included in user and nice.
	for _, s := range fields[1:9] {
		n, err := strconv.ParseUint(s, 10, 64)
		if err != nil {
			return out, err
		}
		out.total += n
	}
	out.idle = number(fields[4]) + number(fields[5])
	return out, nil
}
func parseMemory(data []byte) (uint64, uint64, error) {
	values := map[string]uint64{}
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) >= 2 {
			key := strings.TrimSuffix(f[0], ":")
			if key == "MemTotal" || key == "MemAvailable" {
				n, err := strconv.ParseUint(f[1], 10, 64)
				if err != nil {
					return 0, 0, err
				}
				values[key] = n * 1024
			}
		}
	}
	total, ok := values["MemTotal"]
	available, exists := values["MemAvailable"]
	if !ok || !exists || total == 0 || available > total {
		return 0, 0, fmt.Errorf("MemTotal или MemAvailable недоступны")
	}
	return total, total - available, nil
}
func parseNetwork(data []byte) (map[string]netCounter, error) {
	out := map[string]netCounter{}
	for _, line := range strings.Split(string(data), "\n") {
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		f := strings.Fields(rest)
		if len(f) < 16 {
			return nil, fmt.Errorf("неверный сетевой счётчик")
		}
		if name == "lo" || strings.HasPrefix(name, "veth") || strings.HasPrefix(name, "docker") || strings.HasPrefix(name, "br-") {
			continue
		}
		rx, err := strconv.ParseUint(f[0], 10, 64)
		if err != nil {
			return nil, err
		}
		tx, err := strconv.ParseUint(f[8], 10, 64)
		if err != nil {
			return nil, err
		}
		out[name] = netCounter{rx, tx}
	}
	return out, nil
}
func unescape(s string) string {
	return strings.NewReplacer(`\040`, " ", `\011`, "\t", `\134`, `\`).Replace(s)
}
func (c *Collector) refreshMounts(now time.Time) error {
	data, err := c.read("1/mountinfo")
	if err != nil {
		return err
	}
	mounts := []mount{}
	seen := map[string]bool{}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		left, right, ok := strings.Cut(scanner.Text(), " - ")
		if !ok {
			continue
		}
		a, b := strings.Fields(left), strings.Fields(right)
		if len(a) < 5 || len(b) < 2 {
			continue
		}
		// Real disk-backed filesystems only; root overlay is useful in uncontainerized development.
		supported := map[string]bool{"ext2": true, "ext3": true, "ext4": true, "xfs": true, "btrfs": true, "zfs": true, "vfat": true, "f2fs": true, "overlay": true}
		path := unescape(a[4])
		if !supported[b[0]] || (b[0] == "overlay" && path != "/") || (path != "/" && a[3] != "/") || seen[a[2]] {
			continue
		}
		seen[a[2]] = true
		mounts = append(mounts, mount{unescape(b[1]), path})
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	c.mounts = mounts
	c.mountRefresh = now
	return nil
}
func (c *Collector) Collect(now time.Time) model.Snapshot {
	start := time.Now()
	s := model.Snapshot{Time: now.UTC(), Disks: []model.Disk{}, Networks: []model.Network{}, Errors: map[string]string{}}
	data, err := c.read("stat")
	if err == nil {
		var current cpuCounter
		current, err = parseCPU(data)
		if err == nil {
			if p := c.previousCPU; p != nil && current.total > p.total && current.idle >= p.idle {
				total := current.total - p.total
				idle := current.idle - p.idle
				if idle <= total {
					s.CPU = ptr(100 * float64(total-idle) / float64(total))
				}
			}
			c.previousCPU = &current
		}
	}
	if err != nil {
		s.Errors["cpu"] = err.Error()
		c.previousCPU = nil
	}
	data, err = c.read("meminfo")
	if err == nil {
		s.MemoryTotal, s.MemoryUsed, err = parseMemory(data)
		if err == nil {
			s.Memory = ptr(100 * float64(s.MemoryUsed) / float64(s.MemoryTotal))
		}
	}
	if err != nil {
		s.Errors["memory"] = err.Error()
	}
	data, err = c.read("net/dev")
	if err == nil {
		var current map[string]netCounter
		current, err = parseNetwork(data)
		if err == nil {
			var rx, tx float64
			valid := false
			seconds := now.Sub(c.previousTime).Seconds()
			for name, n := range current {
				item := model.Network{Name: name}
				if old, ok := c.previousNet[name]; ok && seconds > 0 && n.rx >= old.rx && n.tx >= old.tx {
					r, t := float64(n.rx-old.rx)/seconds, float64(n.tx-old.tx)/seconds
					item.RX = &r
					item.TX = &t
					rx += r
					tx += t
					valid = true
				}
				s.Networks = append(s.Networks, item)
			}
			if valid {
				s.RX = &rx
				s.TX = &tx
			}
			c.previousNet = current
			c.previousTime = now
		}
	}
	if err != nil {
		s.Errors["network"] = err.Error()
		c.previousNet = nil
	}
	if c.mountRefresh.IsZero() || now.Sub(c.mountRefresh) > time.Minute {
		if err := c.refreshMounts(now); err != nil {
			s.Errors["disk"] = err.Error()
		}
	}
	var largest float64
	available := false
	for _, m := range c.mounts {
		var st syscall.Statfs_t
		if err := syscall.Statfs(filepath.Join(c.Root, m.path), &st); err != nil {
			s.Errors["disk:"+m.path] = err.Error()
			continue
		}
		total := st.Blocks * uint64(st.Bsize)
		free := st.Bavail * uint64(st.Bsize)
		used := (st.Blocks - st.Bfree) * uint64(st.Bsize)
		if total == 0 {
			continue
		}
		if used+free == 0 {
			continue
		}
		percent := 100 * float64(used) / float64(used+free)
		s.Disks = append(s.Disks, model.Disk{Name: m.device, Mount: m.path, Total: total, Used: used, Percent: percent})
		if percent > largest {
			largest = percent
		}
		available = true
	}
	if available {
		s.Disk = &largest
	} else if _, ok := s.Errors["disk"]; !ok {
		s.Errors["disk"] = "Нет доступных файловых систем"
	}
	if data, err := c.read("loadavg"); err == nil {
		f := strings.Fields(string(data))
		if len(f) > 0 {
			if n, e := strconv.ParseFloat(f[0], 64); e == nil {
				s.Load1 = &n
			}
		}
	}
	sort.Slice(s.Networks, func(i, j int) bool { return s.Networks[i].Name < s.Networks[j].Name })
	s.CollectionMS = float64(time.Since(start).Microseconds()) / 1000
	return s
}
func (c *Collector) Host() model.Host {
	h := model.Host{OS: "Linux"}
	if data, e := c.read("sys/kernel/hostname"); e == nil {
		h.Name = strings.TrimSpace(string(data))
	}
	if data, e := c.read("sys/kernel/osrelease"); e == nil {
		h.Kernel = strings.TrimSpace(string(data))
	}
	if data, e := c.read("cpuinfo"); e == nil {
		h.Cores = strings.Count(string(data), "processor\t:")
	}
	if data, e := c.read("uptime"); e == nil {
		f := strings.Fields(string(data))
		if len(f) > 0 {
			h.Uptime, _ = strconv.ParseFloat(f[0], 64)
		}
	}
	return h
}

// Reset makes the first CPU/network sample after a stop unavailable rather than
// reporting an average across the interval during which monitoring was disabled.
// The scheduler is the sole caller of Collect and Reset.
func (c *Collector) Reset() {
	c.previousCPU = nil
	c.previousNet = nil
	c.previousTime = time.Time{}
}
