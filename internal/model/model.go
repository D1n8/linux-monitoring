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

type Host struct {
	Name   string  `json:"name"`
	OS     string  `json:"os"`
	Kernel string  `json:"kernel"`
	Cores  int     `json:"cores"`
	Uptime float64 `json:"uptime"`
}
