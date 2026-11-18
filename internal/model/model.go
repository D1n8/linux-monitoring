package model

import "time"

type Disk struct {
	Name    string  `json:"name"`
	Mount   string  `json:"mount"`
	Total   uint64  `json:"total"`
	Used    uint64  `json:"used"`
	Percent float64 `json:"percent"`
}
type Network struct {
	Name string   `json:"name"`
	RX   *float64 `json:"rx"`
	TX   *float64 `json:"tx"`
}
type Snapshot struct {
	Time         time.Time         `json:"time"`
	CPU          *float64          `json:"cpu"`
	Memory       *float64          `json:"memory"`
	MemoryTotal  uint64            `json:"memory_total"`
	MemoryUsed   uint64            `json:"memory_used"`
	Disk         *float64          `json:"disk"`
	RX           *float64          `json:"rx"`
	TX           *float64          `json:"tx"`
	Load1        *float64          `json:"load1"`
	Disks        []Disk            `json:"disks"`
	Networks     []Network         `json:"networks"`
	Errors       map[string]string `json:"errors"`
	CollectionMS float64           `json:"collection_ms"`
}

func (s Snapshot) Value(metric string) *float64 {
	switch metric {
	case "cpu":
		return s.CPU
	case "memory":
		return s.Memory
	case "disk":
		return s.Disk
	case "rx":
		return s.RX
	case "tx":
		return s.TX
	}
	return nil
}

type Host struct {
	Name   string  `json:"name"`
	OS     string  `json:"os"`
	Kernel string  `json:"kernel"`
	Cores  int     `json:"cores"`
	Uptime float64 `json:"uptime"`
}
type Rule struct {
	Metric    string  `json:"metric"`
	Threshold float64 `json:"threshold"`
	Duration  int     `json:"duration"`
	Enabled   bool    `json:"enabled"`
}
type Event struct {
	ID        int64      `json:"id"`
	Metric    string     `json:"metric"`
	Threshold float64    `json:"threshold"`
	Started   time.Time  `json:"started"`
	Ended     *time.Time `json:"ended"`
	State     string     `json:"state"`
}
