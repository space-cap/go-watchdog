package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// Target represents a health check target configuration fetched from central HQ.
type Target struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	URL             string `json:"url"`
	IntervalSeconds int    `json:"interval_seconds"`
	TimeoutSeconds  int    `json:"timeout_seconds"`
}

// ReportPayload represents a single health check measurement result reported to central HQ.
type ReportPayload struct {
	TargetID     int64     `json:"target_id"`
	StatusCode   int       `json:"status_code"`
	LatencyMs    int       `json:"latency_ms"`
	IsSuccess    bool      `json:"is_success"`
	ErrorMessage string    `json:"error_message"`
	Timestamp    time.Time `json:"timestamp"`
}

// TargetJob holds the runner goroutine state for a single target.
type TargetJob struct {
	Target Target
	StopCh chan struct{}
}

// CheckerRunner manages polling target list from central server and executing checks.
type CheckerRunner struct {
	portalURL    string
	checkerToken string
	syncInterval time.Duration
	client       *http.Client

	jobsMutex sync.Mutex
	jobs      map[int64]*TargetJob
	stopSync  chan struct{}
}

// NewCheckerRunner creates a new CheckerRunner instance.
func NewCheckerRunner(portalURL, checkerToken string, syncIntervalSec int) *CheckerRunner {
	if syncIntervalSec <= 0 {
		syncIntervalSec = 30
	}

	return &CheckerRunner{
		portalURL:    portalURL,
		checkerToken: checkerToken,
		syncInterval: time.Duration(syncIntervalSec) * time.Second,
		client: &http.Client{
			Timeout: 10 * time.Second,
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			},
		},
		jobs:     make(map[int64]*TargetJob),
		stopSync: make(chan struct{}),
	}
}

// Start begins periodic target synchronization and execution.
func (cr *CheckerRunner) Start() {
	log.Printf("[CheckerRunner] Starting Go Checker engine targeting: %s (Sync every %v)", cr.portalURL, cr.syncInterval)
	cr.syncTargets()

	go func() {
		ticker := time.NewTicker(cr.syncInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				cr.syncTargets()
			case <-cr.stopSync:
				return
			}
		}
	}()
}

// Stop safely terminates all target jobs and synchronization.
func (cr *CheckerRunner) Stop() {
	close(cr.stopSync)

	cr.jobsMutex.Lock()
	defer cr.jobsMutex.Unlock()

	for id, job := range cr.jobs {
		close(job.StopCh)
		delete(cr.jobs, id)
	}
	log.Println("[CheckerRunner] Checker engine stopped cleanly.")
}

func (cr *CheckerRunner) syncTargets() {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/checker/targets", cr.portalURL), nil)
	if err != nil {
		log.Printf("[CheckerRunner] [Error] Failed to create sync request: %v", err)
		return
	}
	req.Header.Set("X-Checker-Token", cr.checkerToken)

	resp, err := cr.client.Do(req)
	if err != nil {
		log.Printf("[CheckerRunner] [Warning] HQ server unreachable: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[CheckerRunner] [Warning] Failed to fetch targets (HTTP %d)", resp.StatusCode)
		return
	}

	var fetchedTargets []Target
	if err := json.NewDecoder(resp.Body).Decode(&fetchedTargets); err != nil {
		log.Printf("[CheckerRunner] [Error] Failed to decode targets JSON: %v", err)
		return
	}

	cr.jobsMutex.Lock()
	defer cr.jobsMutex.Unlock()

	activeTargetIDs := make(map[int64]bool)

	for _, t := range fetchedTargets {
		activeTargetIDs[t.ID] = true
		existingJob, exists := cr.jobs[t.ID]

		if exists {
			// Update target definition if interval or URL changed
			if existingJob.Target.URL != t.URL || existingJob.Target.IntervalSeconds != t.IntervalSeconds || existingJob.Target.TimeoutSeconds != t.TimeoutSeconds {
				close(existingJob.StopCh)
				delete(cr.jobs, t.ID)
				cr.startTargetJob(t)
			}
		} else {
			cr.startTargetJob(t)
		}
	}

	// Remove jobs for targets no longer active in HQ
	for id, job := range cr.jobs {
		if !activeTargetIDs[id] {
			close(job.StopCh)
			delete(cr.jobs, id)
			log.Printf("[CheckerRunner] Stopped target job ID %d (%s)", id, job.Target.Name)
		}
	}
}

func (cr *CheckerRunner) startTargetJob(t Target) {
	stopCh := make(chan struct{})
	job := &TargetJob{
		Target: t,
		StopCh: stopCh,
	}
	cr.jobs[t.ID] = job

	interval := time.Duration(t.IntervalSeconds) * time.Second
	if interval <= 0 {
		interval = 60 * time.Second
	}

	log.Printf("[CheckerRunner] Started target job: %s (%s, interval: %v)", t.Name, t.URL, interval)

	go func(job *TargetJob) {
		// Run initial check immediately
		cr.checkAndReport(job.Target)

		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				cr.checkAndReport(job.Target)
			case <-job.StopCh:
				return
			}
		}
	}(job)
}

func (cr *CheckerRunner) checkAndReport(t Target) {
	timeout := time.Duration(t.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	checkClient := &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	start := time.Now()
	resp, err := checkClient.Get(t.URL)
	latency := time.Since(start)

	var statusCode int
	var isSuccess bool
	var errMsg string

	if err != nil {
		isSuccess = false
		errMsg = err.Error()
	} else {
		defer resp.Body.Close()
		statusCode = resp.StatusCode
		if resp.StatusCode >= 200 && resp.StatusCode < 400 {
			isSuccess = true
		} else {
			isSuccess = false
			errMsg = fmt.Sprintf("HTTP Status Code: %d", resp.StatusCode)
		}
	}

	payload := ReportPayload{
		TargetID:     t.ID,
		StatusCode:   statusCode,
		LatencyMs:    int(latency.Milliseconds()),
		IsSuccess:    isSuccess,
		ErrorMessage: errMsg,
		Timestamp:    time.Now(),
	}

	cr.sendReport([]ReportPayload{payload})
}

func (cr *CheckerRunner) sendReport(reports []ReportPayload) {
	bodyBytes, err := json.Marshal(reports)
	if err != nil {
		log.Printf("[CheckerRunner] [Error] Failed to marshal report payload: %v", err)
		return
	}

	req, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/api/checker/report", cr.portalURL), bytes.NewBuffer(bodyBytes))
	if err != nil {
		log.Printf("[CheckerRunner] [Error] Failed to create report request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Checker-Token", cr.checkerToken)

	resp, err := cr.client.Do(req)
	if err != nil {
		log.Printf("[CheckerRunner] [Warning] Failed to post report to HQ: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[CheckerRunner] [Warning] Report rejected by HQ (HTTP %d)", resp.StatusCode)
	}
}
