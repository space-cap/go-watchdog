package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// HealthTarget represents the configuration of an API to be monitored.
type HealthTarget struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	URL             string    `json:"url"`
	IntervalSeconds int       `json:"interval_seconds"`
	TimeoutSeconds  int       `json:"timeout_seconds"`
	IsActive        int       `json:"is_active"`
	CreatedAt       time.Time `json:"created_at"`
}

// Job manages the execution context and shutdown mechanism for an active check.
type Job struct {
	target   HealthTarget
	stopChan chan struct{}
}

// HealthRunner schedules and runs periodic health checks for all active targets.
type HealthRunner struct {
	store       DataStore
	notifier    *Notifier
	statusCache map[int64]string
	cacheMutex  sync.RWMutex
	jobs        map[int64]*Job
	jobsMutex   sync.Mutex
	stopChan    chan struct{}
}

// NewHealthRunner initializes a HealthRunner instance.
func NewHealthRunner(store DataStore, notifier *Notifier) *HealthRunner {
	return &HealthRunner{
		store:       store,
		notifier:    notifier,
		statusCache: make(map[int64]string),
		jobs:        make(map[int64]*Job),
		stopChan:    make(chan struct{}),
	}
}

// Start kicks off the scheduler daemon and syncs targets.
func (hr *HealthRunner) Start() {
	hr.syncJobs()

	// Sync database configurations with active loops every 10 seconds
	ticker := time.NewTicker(10 * time.Second)
	go func() {
		for {
			select {
			case <-ticker.C:
				hr.syncJobs()
			case <-hr.stopChan:
				ticker.Stop()
				hr.stopAllJobs()
				return
			}
		}
	}()
}

// Stop stops the scheduler daemon cleanly.
func (hr *HealthRunner) Stop() {
	close(hr.stopChan)
}

func (hr *HealthRunner) stopAllJobs() {
	hr.jobsMutex.Lock()
	defer hr.jobsMutex.Unlock()

	for id, job := range hr.jobs {
		close(job.stopChan)
		delete(hr.jobs, id)
	}
	log.Println("[HealthRunner] Stopped all health check jobs cleanly.")
}

func (hr *HealthRunner) syncJobs() {
	hr.jobsMutex.Lock()
	defer hr.jobsMutex.Unlock()

	targets, err := hr.store.GetActiveTargets()
	if err != nil {
		log.Printf("[HealthRunner] [Error] Failed to query active targets: %v", err)
		return
	}

	activeDBTargets := make(map[int64]HealthTarget)
	for _, t := range targets {
		activeDBTargets[t.ID] = t
	}

	// 1. Remove jobs that are deleted, inactive, or had configuration changes
	for id, job := range hr.jobs {
		dbTarget, exists := activeDBTargets[id]
		if !exists || job.target.URL != dbTarget.URL || job.target.IntervalSeconds != dbTarget.IntervalSeconds || job.target.TimeoutSeconds != dbTarget.TimeoutSeconds {
			close(job.stopChan)
			delete(hr.jobs, id)
			log.Printf("[HealthRunner] Stopped/Reset target job: ID %d (%s)", id, job.target.Name)
		}
	}

	// 2. Spawn new jobs
	for id, t := range activeDBTargets {
		if _, running := hr.jobs[id]; !running {
			stopChan := make(chan struct{})
			hr.jobs[id] = &Job{
				target:   t,
				stopChan: stopChan,
			}
			go hr.runCheckLoop(t, stopChan)
			log.Printf("[HealthRunner] Started target job: %s (%s, interval: %ds)", t.Name, t.URL, t.IntervalSeconds)
		}
	}
}

func (hr *HealthRunner) runCheckLoop(t HealthTarget, stopChan chan struct{}) {
	// Execute an initial check immediately on startup
	hr.checkTarget(t)

	ticker := time.NewTicker(time.Duration(t.IntervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			hr.checkTarget(t)
		case <-stopChan:
			return
		}
	}
}

func (hr *HealthRunner) checkTarget(t HealthTarget) {
	// 1. Fetch previous state (cache first, fallback to DB)
	hr.cacheMutex.Lock()
	prevStatus, exists := hr.statusCache[t.ID]
	if !exists {
		prevStatus = hr.getLatestStatusFromDB(t.ID)
		if prevStatus != "" {
			hr.statusCache[t.ID] = prevStatus
		} else {
			// 최초 등록되어 이전 상태 이력이 없는 타겟은
			// 기본 상태를 "ONLINE"으로 가정하여 첫 감시 실패 시 즉각 알림이 가도록 조치
			prevStatus = "ONLINE"
			hr.statusCache[t.ID] = prevStatus
		}
	}
	hr.cacheMutex.Unlock()

	// 2. Perform HTTP call (allowing self-signed/development TLS certificates)
	client := &http.Client{
		Timeout: time.Duration(t.TimeoutSeconds) * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	start := time.Now()
	resp, err := client.Get(t.URL)
	latency := time.Since(start)

	var statusCode int
	var isSuccess int
	var errMsg string

	if err != nil {
		isSuccess = 0
		errMsg = err.Error()
	} else {
		defer resp.Body.Close()
		statusCode = resp.StatusCode
		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			isSuccess = 1
		} else {
			isSuccess = 0
			errMsg = fmt.Sprintf("HTTP Status Code: %d", resp.StatusCode)
		}
	}

	latencyMs := int(latency.Milliseconds())

	// 3. Write results to database
	dbErr := hr.store.SaveHealthLog(t.ID, statusCode, latencyMs, isSuccess == 1, errMsg)
	if dbErr != nil {
		log.Printf("[HealthRunner] [Error] Failed to insert health log for %s: %v", t.Name, dbErr)
	}

	// 4. Update state and trigger alerts on status transition
	currentStatus := "OFFLINE"
	if isSuccess == 1 {
		currentStatus = "ONLINE"
	}

	hr.cacheMutex.Lock()
	hr.statusCache[t.ID] = currentStatus
	hr.cacheMutex.Unlock()

	// Only send alert if we had a prior recorded status and it transition (ONLINE <-> OFFLINE)
	if prevStatus != "" && prevStatus != currentStatus {
		log.Printf("[HealthRunner] Status Changed for %s: %s -> %s (Latency: %dms)", t.Name, prevStatus, currentStatus, latencyMs)
		hr.notifier.SendAlert(t.Name, t.URL, currentStatus, latencyMs, statusCode, errMsg)
	}
}

func (hr *HealthRunner) getLatestStatusFromDB(targetID int64) string {
	status, err := hr.store.GetLatestTargetStatus(targetID)
	if err != nil {
		return ""
	}
	return status
}
