package main

import (
	"embed"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"go-watchdog/common"
)

//go:embed templates/*
var templatesFS embed.FS

// Server handles all HTTP routing, authentication, and database dependency mapping.
type Server struct {
	store       DataStore
	authToken   string
	cacheMutex  sync.RWMutex
	metricCache map[string]*common.Metric
}

// NewServer initializes a new Server instance.
func NewServer(store DataStore, authToken string) *Server {
	return &Server{
		store:       store,
		authToken:   authToken,
		metricCache: make(map[string]*common.Metric),
	}
}

// ServeDashboard serves the embedded dashboard HTML page at the root route.
func (s *Server) ServeDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// Check if token query param matches admin token
	tokenParam := r.URL.Query().Get("token")
	if tokenParam == s.authToken {
		http.SetCookie(w, &http.Cookie{
			Name:     "session_token",
			Value:    s.authToken,
			Path:     "/",
			HttpOnly: true, // Prevent XSS theft
			MaxAge:   86400, // 1 day
		})
		// Redirect to root without query param to clean browser URL
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	htmlContent, err := templatesFS.ReadFile("templates/dashboard.html")
	if err != nil {
		log.Printf("[Server] [Error] Failed to read embedded dashboard.html: %v", err)
		http.Error(w, "Internal Server Error: Dashboard files missing", http.StatusInternalServerError)
		return
	}

	// Inject IS_ADMIN flag script
	isAdminStr := "false"
	if s.isAdmin(r) {
		isAdminStr = "true"
	}
	flagScript := fmt.Sprintf("<script>const IS_ADMIN = %s;</script>", isAdminStr)
	injectedHTML := strings.Replace(string(htmlContent), "<!-- ADMIN_FLAG -->", flagScript, 1)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(injectedHTML))
}

// isAdmin checks if the request has a valid admin session cookie.
func (s *Server) isAdmin(r *http.Request) bool {
	cookie, err := r.Cookie("session_token")
	if err != nil {
		return false
	}
	return cookie.Value == s.authToken
}

// TokenAuthMiddleware authenticates agent report requests by verifying the X-Agent-Token header.
func (s *Server) TokenAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("X-Agent-Token")
		if token != s.authToken {
			log.Printf("[Server] [Warning] Unauthorized metrics submission attempt from IP: %s", r.RemoteAddr)
			http.Error(w, "Unauthorized: Invalid or missing X-Agent-Token", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// HandlePostMetrics receives system resource reports from agents and saves them to the DB.
func (s *Server) HandlePostMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var m common.Metric
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&m); err != nil {
		http.Error(w, "Bad Request: Failed to parse metric JSON payload", http.StatusBadRequest)
		return
	}

	if m.AgentID == "" {
		http.Error(w, "Bad Request: agent_id is a required field", http.StatusBadRequest)
		return
	}

	// Default to current server time if the agent did not supply a valid timestamp
	if m.Timestamp.IsZero() {
		m.Timestamp = time.Now()
	}

	if err := s.store.SaveMetric(&m); err != nil {
		log.Printf("[Server] [Error] Failed to save metrics for %s: %v", m.AgentID, err)
		http.Error(w, "Internal Server Error: Failed to store metrics database side", http.StatusInternalServerError)
		return
	}

	// Update in-memory cache for instant zero-DB dashboard response
	s.cacheMutex.Lock()
	s.metricCache[m.AgentID] = &m
	s.cacheMutex.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(`{"status":"success"}`))
}

// HandleGetStatus returns the status of all registered servers including a status flag (ONLINE/OFFLINE).
func (s *Server) HandleGetStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check in-memory cache first to avoid DB query overhead
	s.cacheMutex.RLock()
	hasCache := len(s.metricCache) > 0
	cachedList := make([]*common.Metric, 0, len(s.metricCache))
	if hasCache {
		for _, m := range s.metricCache {
			cachedList = append(cachedList, m)
		}
	}
	s.cacheMutex.RUnlock()

	var metrics []*common.Metric
	var err error

	if hasCache {
		metrics = cachedList
	} else {
		// Fallback to DB query on cold startup if cache is empty
		metrics, err = s.store.GetLatestMetrics()
		if err != nil {
			log.Printf("[Server] [Error] Failed to fetch latest status: %v", err)
			http.Error(w, "Internal Server Error: Database retrieval error", http.StatusInternalServerError)
			return
		}
		// Populate cache from DB query result
		s.cacheMutex.Lock()
		for _, m := range metrics {
			s.metricCache[m.AgentID] = m
		}
		s.cacheMutex.Unlock()
	}

	// Response DTO containing agent metrics and its computed status
	type AgentStatusResponse struct {
		AgentID    string            `json:"agent_id"`
		CPUPercent float64           `json:"cpu_percent"`
		MemTotalGB float64           `json:"mem_total_gb"`
		MemUsedGB  float64           `json:"mem_used_gb"`
		MemPercent float64           `json:"mem_percent"`
		Disks      []common.DiskInfo `json:"disks"`
		Timestamp  time.Time         `json:"timestamp"`
		Status     string            `json:"status"` // ONLINE or OFFLINE
	}

	response := make([]AgentStatusResponse, 0, len(metrics))
	for _, m := range metrics {
		status := "ONLINE"
		// If last contact was more than 30 seconds ago, mark the agent as OFFLINE
		if time.Since(m.Timestamp) > 30*time.Second {
			status = "OFFLINE"
		}

		response = append(response, AgentStatusResponse{
			AgentID:    m.AgentID,
			CPUPercent: m.CPUPercent,
			MemTotalGB: m.MemTotalGB,
			MemUsedGB:  m.MemUsedGB,
			MemPercent: m.MemPercent,
			Disks:      m.Disks,
			Timestamp:  m.Timestamp,
			Status:     status,
		})
	}

	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = json.NewEncoder(w).Encode(response)
}

// HandleGetHealthTargets returns all registered health check targets
func (s *Server) HandleGetHealthTargets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	targets, err := s.store.GetHealthTargets()
	if err != nil {
		log.Printf("[Server] [Error] Failed to query health targets: %v", err)
		http.Error(w, "Database query error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(targets)
}

// HandlePostHealthTarget creates a new health check target
func (s *Server) HandlePostHealthTarget(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.isAdmin(r) {
		http.Error(w, "Unauthorized: Admin session required", http.StatusUnauthorized)
		return
	}

	var req struct {
		Name            string `json:"name"`
		URL             string `json:"url"`
		IntervalSeconds int    `json:"interval_seconds"`
		TimeoutSeconds  int    `json:"timeout_seconds"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad Request: Invalid JSON body", http.StatusBadRequest)
		return
	}

	if req.Name == "" || req.URL == "" {
		http.Error(w, "Bad Request: name and url are required", http.StatusBadRequest)
		return
	}

	if req.IntervalSeconds <= 0 {
		req.IntervalSeconds = 60
	}
	if req.TimeoutSeconds <= 0 {
		req.TimeoutSeconds = 5
	}

	id, err := s.store.SaveHealthTarget(req.Name, req.URL, req.IntervalSeconds, req.TimeoutSeconds)
	if err != nil {
		log.Printf("[Server] [Error] Failed to insert health target: %v", err)
		http.Error(w, "Database insert error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_, _ = w.Write([]byte(fmt.Sprintf(`{"status":"created","id":%d}`, id)))
}

// HandleDeleteHealthTarget deletes a health check target by ID
func (s *Server) HandleDeleteHealthTarget(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if !s.isAdmin(r) {
		http.Error(w, "Unauthorized: Admin session required", http.StatusUnauthorized)
		return
	}

	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		http.Error(w, "Bad Request: id query parameter is required", http.StatusBadRequest)
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "Bad Request: invalid id format", http.StatusBadRequest)
		return
	}

	err = s.store.DeleteHealthTarget(id)
	if err != nil {
		log.Printf("[Server] [Error] Failed to delete health target: %v", err)
		http.Error(w, "Database delete error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"status":"deleted"}`))
}

type HealthTargetStatus struct {
	ID             int64     `json:"id"`
	Name           string    `json:"name"`
	URL            string    `json:"url"`
	Interval       int       `json:"interval_seconds"`
	Status         string    `json:"status"` // ONLINE or OFFLINE
	LastCheck      time.Time `json:"last_check"`
	LastLatencyMs  int       `json:"last_latency_ms"`
	LastStatusCode int       `json:"last_status_code"`
	ErrorMessage   string    `json:"error_message,omitempty"`
	History        []int     `json:"history"` // 1: success, 0: fail (최신 10개)
}

// HandleGetHealthStatus returns the latest status and check history of all health targets
func (s *Server) HandleGetHealthStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response, err := s.store.GetHealthTargetsStatus()
	if err != nil {
		log.Printf("[Server] [Error] Failed to query active targets: %v", err)
		http.Error(w, "Database query error", http.StatusInternalServerError)
		return
	}

	// Apply admin privilege restriction (hide URLs for non-admins)
	isAdmin := s.isAdmin(r)
	for i := range response {
		if !isAdmin {
			response[i].URL = "Hidden (Admin Only)"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = json.NewEncoder(w).Encode(response)
}
