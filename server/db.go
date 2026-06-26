package main

import (
	"database/sql"
	"fmt"
	"time"

	"go-watchdog/common"

	_ "modernc.org/sqlite"
)

// InitDB initializes the SQLite database, sets up tables, and optimizes connection pragmas.
func InitDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pragmas for performance and safety
	_, err = db.Exec("PRAGMA foreign_keys = ON;")
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	_, err = db.Exec("PRAGMA journal_mode = WAL;")
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	_, err = db.Exec("PRAGMA busy_timeout = 5000;")
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to set busy timeout: %w", err)
	}

	// Create tables
	queryCreateMetricsTable := `
	CREATE TABLE IF NOT EXISTS metrics (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		agent_id TEXT NOT NULL,
		cpu_percent REAL NOT NULL,
		mem_total_gb REAL NOT NULL,
		mem_used_gb REAL NOT NULL,
		mem_percent REAL NOT NULL,
		timestamp DATETIME NOT NULL
	);`
	if _, err := db.Exec(queryCreateMetricsTable); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create metrics table: %w", err)
	}

	queryCreateDiskMetricsTable := `
	CREATE TABLE IF NOT EXISTS disk_metrics (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		metric_id INTEGER NOT NULL,
		path TEXT NOT NULL,
		total_gb REAL NOT NULL,
		used_gb REAL NOT NULL,
		free_gb REAL NOT NULL,
		percent REAL NOT NULL,
		FOREIGN KEY(metric_id) REFERENCES metrics(id) ON DELETE CASCADE
	);`
	if _, err := db.Exec(queryCreateDiskMetricsTable); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create disk_metrics table: %w", err)
	}

	// Create indices to speed up queries
	queryCreateIndex := `CREATE INDEX IF NOT EXISTS idx_metrics_agent_timestamp ON metrics(agent_id, timestamp);`
	if _, err := db.Exec(queryCreateIndex); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create index: %w", err)
	}

	return db, nil
}

// SaveMetric saves a system performance metric payload to the database in a single transaction.
func SaveMetric(db *sql.DB, m *common.Metric) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Insert parent metric row
	res, err := tx.Exec(`
		INSERT INTO metrics (agent_id, cpu_percent, mem_total_gb, mem_used_gb, mem_percent, timestamp)
		VALUES (?, ?, ?, ?, ?, ?)
	`, m.AgentID, m.CPUPercent, m.MemTotalGB, m.MemUsedGB, m.MemPercent, m.Timestamp)
	if err != nil {
		return fmt.Errorf("failed to insert metric row: %w", err)
	}

	metricID, err := res.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to retrieve last insert id: %w", err)
	}

	// Insert child disk metric rows
	for _, disk := range m.Disks {
		_, err := tx.Exec(`
			INSERT INTO disk_metrics (metric_id, path, total_gb, used_gb, free_gb, percent)
			VALUES (?, ?, ?, ?, ?, ?)
		`, metricID, disk.Path, disk.TotalGB, disk.UsedGB, disk.FreeGB, disk.Percent)
		if err != nil {
			return fmt.Errorf("failed to insert disk metric row: %w", err)
		}
	}

	return tx.Commit()
}

// GetLatestMetrics fetches the most recent metric entry for each monitored agent.
func GetLatestMetrics(db *sql.DB) ([]*common.Metric, error) {
	rows, err := db.Query(`
		SELECT id, agent_id, cpu_percent, mem_total_gb, mem_used_gb, mem_percent, timestamp
		FROM metrics
		WHERE id IN (
			SELECT MAX(id)
			FROM metrics
			GROUP BY agent_id
		)
		ORDER BY agent_id ASC
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to query latest metrics: %w", err)
	}
	defer rows.Close()

	var metrics []*common.Metric
	for rows.Next() {
		var id int64
		var m common.Metric
		err := rows.Scan(&id, &m.AgentID, &m.CPUPercent, &m.MemTotalGB, &m.MemUsedGB, &m.MemPercent, &m.Timestamp)
		if err != nil {
			return nil, fmt.Errorf("failed to scan metric: %w", err)
		}

		disks, err := getDiskMetricsForID(db, id)
		if err != nil {
			return nil, fmt.Errorf("failed to get disk metrics for id %d: %w", id, err)
		}
		m.Disks = disks

		metrics = append(metrics, &m)
	}

	return metrics, nil
}

// getDiskMetricsForID helper function retrieves all disk partition data for a given metric record ID.
func getDiskMetricsForID(db *sql.DB, metricID int64) ([]common.DiskInfo, error) {
	rows, err := db.Query(`
		SELECT path, total_gb, used_gb, free_gb, percent
		FROM disk_metrics
		WHERE metric_id = ?
	`, metricID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var disks []common.DiskInfo
	for rows.Next() {
		var d common.DiskInfo
		err := rows.Scan(&d.Path, &d.TotalGB, &d.UsedGB, &d.FreeGB, &d.Percent)
		if err != nil {
			return nil, err
		}
		disks = append(disks, d)
	}

	return disks, nil
}

// CleanupOldMetrics deletes all metric records that are older than the specified retention days.
// Relies on SQLite ON DELETE CASCADE to automatically clean up disk_metrics entries.
func CleanupOldMetrics(db *sql.DB, retentionDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	res, err := db.Exec(`
		DELETE FROM metrics
		WHERE timestamp < ?
	`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to clean up old metrics: %w", err)
	}

	return res.RowsAffected()
}
