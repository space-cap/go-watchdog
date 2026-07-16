package main

import (
	"go-watchdog/common"
)

// DataStore defines the interface for all database operations.
type DataStore interface {
	// Close closes the database connection.
	Close() error

	// CleanupOldMetrics deletes old metrics and health checks based on retention days.
	CleanupOldMetrics(retentionDays int) (int64, error)

	// SaveMetric stores system metrics report from an agent.
	SaveMetric(m *common.Metric) error

	// GetLatestMetrics fetches the most recent metric entry for each monitored agent.
	GetLatestMetrics() ([]*common.Metric, error)

	// GetHealthTargets fetches all configured health targets.
	GetHealthTargets() ([]HealthTarget, error)

	// SaveHealthTarget creates a new health check target.
	SaveHealthTarget(name, url string, interval, timeout int) (int64, error)

	// DeleteHealthTarget deletes a health check target by ID.
	DeleteHealthTarget(id int64) error

	// GetActiveTargets fetches all active health check targets.
	GetActiveTargets() ([]HealthTarget, error)

	// SaveHealthLog records the result of a health check.
	SaveHealthLog(targetID int64, statusCode int, latencyMs int, isSuccess bool, errMsg string) error

	// GetLatestTargetStatus returns the last recorded status (ONLINE/OFFLINE) for a target.
	GetLatestTargetStatus(targetID int64) (string, error)

	// GetHealthTargetsStatus returns the status and recent check history for all active targets.
	GetHealthTargetsStatus() ([]HealthTargetStatus, error)
}
