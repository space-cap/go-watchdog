package main

import (
	"fmt"
	"time"

	"go-watchdog/common"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
)

const bytesInGB = 1024.0 * 1024.0 * 1024.0

// CollectMetrics gathers the current system performance metrics.
func CollectMetrics(agentID string) (*common.Metric, error) {
	// 1. CPU Usage
	// Passing 0 calculates the utilization since the last call (non-blocking)
	cpuPercentages, err := cpu.Percent(0, false)
	var cpuPercent float64
	if err == nil && len(cpuPercentages) > 0 {
		cpuPercent = cpuPercentages[0]
	}

	// 2. Memory Usage
	vMem, err := mem.VirtualMemory()
	if err != nil {
		return nil, fmt.Errorf("failed to collect memory info: %w", err)
	}

	memTotalGB := float64(vMem.Total) / bytesInGB
	memUsedGB := float64(vMem.Used) / bytesInGB
	memPercent := vMem.UsedPercent

	// 3. Disk Usage
	partitions, err := disk.Partitions(false)
	var disks []common.DiskInfo
	if err == nil {
		for _, p := range partitions {
			// Query disk usage of each partition (Skip partition if errors occur, e.g. CD-ROM or unmounted drives)
			usage, err := disk.Usage(p.Mountpoint)
			if err != nil {
				continue
			}

			disks = append(disks, common.DiskInfo{
				Path:    p.Mountpoint,
				TotalGB: float64(usage.Total) / bytesInGB,
				UsedGB:  float64(usage.Used) / bytesInGB,
				FreeGB:  float64(usage.Free) / bytesInGB,
				Percent: usage.UsedPercent,
			})
		}
	}

	return &common.Metric{
		AgentID:    agentID,
		CPUPercent: cpuPercent,
		MemTotalGB: memTotalGB,
		MemUsedGB:  memUsedGB,
		MemPercent: memPercent,
		Disks:      disks,
		Timestamp:  time.Now(),
	}, nil
}
