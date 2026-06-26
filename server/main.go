package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// 1. CLI flags definition
	configPath := flag.String("config", "config.json", "Path to the server configuration file (JSON)")
	port := flag.Int("port", 0, "HTTP server port to bind to (overrides config)")
	token := flag.String("token", "", "Authorization token for agent data reporting (overrides config)")
	dbPath := flag.String("db", "", "SQLite database file location (overrides config)")
	retentionDays := flag.Int("retention", 0, "Metric data storage retention limit in days (overrides config)")
	flag.Parse()

	log.Println("[Server] Initializing go-watchdog backend server...")

	// 2. Load configuration
	var cfg Config
	if _, err := os.Stat(*configPath); err == nil {
		loadedCfg, err := LoadConfig(*configPath)
		if err != nil {
			log.Fatalf("[Server] [Fatal] Failed to load config from %s: %v", *configPath, err)
		}
		cfg = *loadedCfg
		log.Printf("[Server] Configuration loaded from: %s", *configPath)
	} else {
		// Default settings if file does not exist
		cfg = Config{
			Port:          9090,
			AuthToken:     "watchdog-secret-token",
			DBPath:        "monitoring.db",
			RetentionDays: 14,
		}
		log.Printf("[Server] Configuration file %s not found. Using default values.", *configPath)
	}

	// Override with explicitly set CLI flags
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "port":
			cfg.Port = *port
		case "token":
			cfg.AuthToken = *token
		case "db":
			cfg.DBPath = *dbPath
		case "retention":
			cfg.RetentionDays = *retentionDays
		}
	})

	// 3. Initialize Database
	db, err := InitDB(cfg.DBPath)
	if err != nil {
		log.Fatalf("[Server] [Fatal] Database initialization failed: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("[Server] [Error] Failed to close database cleanly: %v", err)
		} else {
			log.Println("[Server] Database connection closed successfully.")
		}
	}()
	log.Printf("[Server] Database initialized at: %s (Retention: %d days)", cfg.DBPath, cfg.RetentionDays)

	// 4. Setup Server Handler
	srv := NewServer(db, cfg.AuthToken)

	mux := http.NewServeMux()
	mux.HandleFunc("/", srv.ServeDashboard)
	mux.HandleFunc("/api/status", srv.HandleGetStatus)
	mux.HandleFunc("/api/metrics", srv.TokenAuthMiddleware(srv.HandlePostMetrics))

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: mux,
	}

	// 5. Background Metrics Retention Cleaner Daemon
	cleanupTicker := time.NewTicker(1 * time.Hour)
	defer cleanupTicker.Stop()

	// Proactively run an initial cleanup on startup
	if affected, err := CleanupOldMetrics(db, cfg.RetentionDays); err != nil {
		log.Printf("[Server] [Warning] Failed to run initial database cleanup: %v", err)
	} else if affected > 0 {
		log.Printf("[Server] Startup cleanup deleted %d expired metric records.", affected)
	}

	go func() {
		log.Println("[Server] Background database retention cleaner daemon started.")
		for range cleanupTicker.C {
			affected, err := CleanupOldMetrics(db, cfg.RetentionDays)
			if err != nil {
				log.Printf("[Server] [Error] Background database cleanup failed: %v", err)
			} else if affected > 0 {
				log.Printf("[Server] Background cleanup deleted %d expired metric records.", affected)
			}
		}
	}()

	// 6. Graceful Shutdown & Signal Handlers
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("[Server] Ingestion and Dashboard HTTP server running on port %d...", cfg.Port)
		if err := httpServer.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("[Server] [Fatal] HTTP server crash: %v", err)
		}
	}()

	// Block until a termination signal is caught
	sig := <-shutdownChan
	log.Printf("[Server] Signal %v intercepted. Commencing graceful shutdown procedures...", sig)

	// Stop background cleaner first by exiting scope & running defers, and shut down http server with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("[Server] [Error] HTTP server shutdown failed: %v", err)
	} else {
		log.Println("[Server] HTTP server stopped serving requests.")
	}
}
