package main

import (
	"database/sql"
	"embed"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"go-watchdog/common"
)

// Declare embed variable to embed dashboard HTML directly into the server binary.
// This ensures go-watchdog operates as a single executable without filesystem dependencies.
//
//go:embed templates/dashboard.html
var templatesFS embed.FS

// Server handles all HTTP routing, authentication, and database dependency mapping.
type Server struct {
	db        *sql.DB
	authToken string
}

// NewServer initializes a new Server instance.
func NewServer(db *sql.DB, authToken string) *Server {
	return &Server{
		db:        db,
		authToken: authToken,
	}
}

// ServeDashboard serves the embedded dashboard HTML page at the root route.
func (s *Server) ServeDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	htmlContent, err := templatesFS.ReadFile("templates/dashboard.html")
	if err != nil {
		log.Printf("[Server] [Error] Failed to read embedded dashboard.html: %v", err)
		http.Error(w, "Internal Server Error: Dashboard files missing", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(htmlContent)
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

	if err := SaveMetric(s.db, &m); err != nil {
		log.Printf("[Server] [Error] Failed to save metrics for %s: %v", m.AgentID, err)
		http.Error(w, "Internal Server Error: Failed to store metrics database side", http.StatusInternalServerError)
		return
	}

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

	metrics, err := GetLatestMetrics(s.db)
	if err != nil {
		log.Printf("[Server] [Error] Failed to fetch latest status: %v", err)
		http.Error(w, "Internal Server Error: Database retrieval error", http.StatusInternalServerError)
		return
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

	w.Header().Set("Content-Type", "application/json")
	// Allow cross-origin requests for custom integration setups if required
	w.Header().Set("Access-Control-Allow-Origin", "*")
	_ = json.NewEncoder(w).Encode(response)
}
