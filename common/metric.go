package common

import "time"

// DiskInfo represents the resource status of a specific disk drive.
type DiskInfo struct {
	Path    string  `json:"path"`    // e.g., "C:", "D:"
	TotalGB float64 `json:"total_gb"`
	UsedGB  float64 `json:"used_gb"`
	FreeGB  float64 `json:"free_gb"`
	Percent float64 `json:"percent"` // Disk usage percentage
}

// Metric represents the system resource payload collected and sent by the agent.
type Metric struct {
	AgentID    string     `json:"agent_id"`    // Configured unique identifier for the agent
	CPUPercent float64    `json:"cpu_percent"` // Overall CPU usage percentage
	MemTotalGB float64    `json:"mem_total_gb"`
	MemUsedGB  float64    `json:"mem_used_gb"`
	MemPercent float64    `json:"mem_percent"` // Memory usage percentage
	Disks      []DiskInfo `json:"disks"`       // Slice containing metrics for each disk partition
	Timestamp  time.Time  `json:"timestamp"`   // The time when metric was captured
}
