package main

import (
	"database/sql"
	"fmt"
	"time"

	"go-watchdog/common"

	_ "modernc.org/sqlite"
)

// SQLiteStore manages connection to a local SQLite database file and implements DataStore interface.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore initializes the SQLite database, sets up tables, configures connection pool, and returns SQLiteStore.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	// Configure DSN to enable automatic time scanning and timezone parsing for modernc.org/sqlite
	dsn := fmt.Sprintf("%s?_texttotime=1&_time_format=sqlite", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool limits to prevent deadlocks on nested queries
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(10)

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

	queryCreateMetricsTimestampIndex := `CREATE INDEX IF NOT EXISTS idx_metrics_timestamp ON metrics(timestamp);`
	if _, err := db.Exec(queryCreateMetricsTimestampIndex); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create metrics timestamp index: %w", err)
	}

	// Create health_targets table
	queryCreateHealthTargetsTable := `
	CREATE TABLE IF NOT EXISTS health_targets (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		url TEXT NOT NULL UNIQUE,
		interval_seconds INTEGER NOT NULL DEFAULT 60,
		timeout_seconds INTEGER NOT NULL DEFAULT 5,
		is_active INTEGER NOT NULL DEFAULT 1,
		created_at DATETIME NOT NULL
	);`
	if _, err := db.Exec(queryCreateHealthTargetsTable); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create health_targets table: %w", err)
	}

	// Create health_logs table
	queryCreateHealthLogsTable := `
	CREATE TABLE IF NOT EXISTS health_logs (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		target_id INTEGER NOT NULL,
		status_code INTEGER,
		latency_ms INTEGER,
		is_success INTEGER NOT NULL,
		error_message TEXT,
		timestamp DATETIME NOT NULL,
		FOREIGN KEY(target_id) REFERENCES health_targets(id) ON DELETE CASCADE
	);`
	if _, err := db.Exec(queryCreateHealthLogsTable); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create health_logs table: %w", err)
	}

	// Create index for health_logs to speed up status query
	queryCreateHealthLogsIndex := `CREATE INDEX IF NOT EXISTS idx_health_logs_target_time ON health_logs(target_id, timestamp DESC);`
	if _, err := db.Exec(queryCreateHealthLogsIndex); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create health_logs index: %w", err)
	}

	queryCreateHealthLogsTimestampIndex := `CREATE INDEX IF NOT EXISTS idx_health_logs_timestamp ON health_logs(timestamp);`
	if _, err := db.Exec(queryCreateHealthLogsTimestampIndex); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create health_logs timestamp index: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// SaveMetric saves a system performance metric payload to the database in a single transaction.
func (s *SQLiteStore) SaveMetric(m *common.Metric) error {
	tx, err := s.db.Begin()
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
func (s *SQLiteStore) GetLatestMetrics() ([]*common.Metric, error) {
	rows, err := s.db.Query(`
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

		disks, err := s.getDiskMetricsForID(id)
		if err != nil {
			return nil, fmt.Errorf("failed to get disk metrics for id %d: %w", id, err)
		}
		m.Disks = disks

		metrics = append(metrics, &m)
	}

	return metrics, nil
}

// getDiskMetricsForID helper function retrieves all disk partition data for a given metric record ID.
func (s *SQLiteStore) getDiskMetricsForID(metricID int64) ([]common.DiskInfo, error) {
	rows, err := s.db.Query(`
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

// CleanupOldMetrics deletes all metric and health check records that are older than the specified retention days.
// Relies on SQLite ON DELETE CASCADE to automatically clean up disk_metrics entries.
func (s *SQLiteStore) CleanupOldMetrics(retentionDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays)
	res, err := s.db.Exec(`
		DELETE FROM metrics
		WHERE timestamp < ?
	`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to clean up old metrics: %w", err)
	}

	affectedMetrics, _ := res.RowsAffected()

	res2, err := s.db.Exec(`
		DELETE FROM health_logs
		WHERE timestamp < ?
	`, cutoff)
	if err != nil {
		return affectedMetrics, fmt.Errorf("failed to clean up old health logs: %w", err)
	}

	affectedLogs, _ := res2.RowsAffected()

	return affectedMetrics + affectedLogs, nil
}

// GetHealthTargets fetches all configured health targets.
func (s *SQLiteStore) GetHealthTargets() ([]HealthTarget, error) {
	rows, err := s.db.Query("SELECT id, name, url, interval_seconds, timeout_seconds, is_active, created_at FROM health_targets ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	targets := make([]HealthTarget, 0)
	for rows.Next() {
		var t HealthTarget
		if err := rows.Scan(&t.ID, &t.Name, &t.URL, &t.IntervalSeconds, &t.TimeoutSeconds, &t.IsActive, &t.CreatedAt); err != nil {
			return nil, err
		}
		targets = append(targets, t)
	}
	return targets, nil
}

// SaveHealthTarget creates a new health check target.
func (s *SQLiteStore) SaveHealthTarget(name, url string, interval, timeout int) (int64, error) {
	res, err := s.db.Exec(`
		INSERT INTO health_targets (name, url, interval_seconds, timeout_seconds, is_active, created_at)
		VALUES (?, ?, ?, ?, 1, ?)
	`, name, url, interval, timeout, time.Now())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// DeleteHealthTarget deletes a health check target by ID.
func (s *SQLiteStore) DeleteHealthTarget(id int64) error {
	_, err := s.db.Exec("DELETE FROM health_targets WHERE id = ?", id)
	return err
}

// GetActiveTargets fetches all active health check targets.
func (s *SQLiteStore) GetActiveTargets() ([]HealthTarget, error) {
	rows, err := s.db.Query("SELECT id, name, url, interval_seconds, timeout_seconds, is_active, created_at FROM health_targets WHERE is_active = 1 ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	targets := make([]HealthTarget, 0)
	for rows.Next() {
		var t HealthTarget
		if err := rows.Scan(&t.ID, &t.Name, &t.URL, &t.IntervalSeconds, &t.TimeoutSeconds, &t.IsActive, &t.CreatedAt); err != nil {
			return nil, err
		}
		targets = append(targets, t)
	}
	return targets, nil
}

// SaveHealthLog records the result of a health check.
func (s *SQLiteStore) SaveHealthLog(targetID int64, statusCode int, latencyMs int, isSuccess bool, errMsg string) error {
	var isSuccessInt int
	if isSuccess {
		isSuccessInt = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO health_logs (target_id, status_code, latency_ms, is_success, error_message, timestamp)
		VALUES (?, ?, ?, ?, ?, ?)
	`, targetID, sql.NullInt64{Int64: int64(statusCode), Valid: statusCode > 0}, latencyMs, isSuccessInt, errMsg, time.Now())
	return err
}

// GetLatestTargetStatus returns the last recorded status (ONLINE/OFFLINE) for a target.
func (s *SQLiteStore) GetLatestTargetStatus(targetID int64) (string, error) {
	var isSuccess int
	err := s.db.QueryRow(`
		SELECT is_success FROM health_logs
		WHERE target_id = ?
		ORDER BY timestamp DESC
		LIMIT 1
	`, targetID).Scan(&isSuccess)

	if err != nil {
		return "", err
	}

	if isSuccess == 1 {
		return "ONLINE", nil
	}
	return "OFFLINE", nil
}

// GetHealthTargetsStatus returns the status and recent check history for all active targets.
func (s *SQLiteStore) GetHealthTargetsStatus() ([]HealthTargetStatus, error) {
	rows, err := s.db.Query("SELECT id, name, url, interval_seconds FROM health_targets WHERE is_active = 1 ORDER BY created_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	response := make([]HealthTargetStatus, 0)

	for rows.Next() {
		var status HealthTargetStatus
		if err := rows.Scan(&status.ID, &status.Name, &status.URL, &status.Interval); err != nil {
			return nil, err
		}

		var logID int64
		var statusCode sql.NullInt64
		var latencyMs int
		var isSuccess int
		var errMsg sql.NullString
		var timestamp time.Time

		err := s.db.QueryRow(`
			SELECT id, status_code, latency_ms, is_success, error_message, timestamp
			FROM health_logs
			WHERE target_id = ?
			ORDER BY timestamp DESC
			LIMIT 1
		`, status.ID).Scan(&logID, &statusCode, &latencyMs, &isSuccess, &errMsg, &timestamp)

		if err == sql.ErrNoRows {
			status.Status = "PENDING"
			status.History = make([]int, 0)
		} else if err != nil {
			return nil, err
		} else {
			status.LastCheck = timestamp
			status.LastLatencyMs = latencyMs
			status.LastStatusCode = int(statusCode.Int64)
			status.ErrorMessage = errMsg.String
			if isSuccess == 1 {
				status.Status = "ONLINE"
			} else {
				status.Status = "OFFLINE"
			}

			historyRows, err := s.db.Query(`
				SELECT is_success
				FROM health_logs
				WHERE target_id = ?
				ORDER BY timestamp DESC
				LIMIT 10
			`, status.ID)
			if err == nil {
				history := make([]int, 0, 10)
				for historyRows.Next() {
					var hSuccess int
					if err := historyRows.Scan(&hSuccess); err == nil {
						history = append(history, hSuccess)
					}
				}
				historyRows.Close()

				for i, j := 0, len(history)-1; i < j; i, j = i+1, j-1 {
					history[i], history[j] = history[j], history[i]
				}
				status.History = history
			} else {
				status.History = make([]int, 0)
			}
		}

		response = append(response, status)
	}

	return response, nil
}
