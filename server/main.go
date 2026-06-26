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
	port := flag.Int("port", 9090, "HTTP server port to bind to")
	token := flag.String("token", "watchdog-secret-token", "Authorization token for agent data reporting")
	dbPath := flag.String("db", "monitoring.db", "SQLite database file location")
	retentionDays := flag.Int("retention", 14, "Metric data storage retention limit in days")
	flag.Parse()

	log.Println("[Server] Initializing go-watchdog backend server...")

	// 2. Initialize Database
	db, err := InitDB(*dbPath)
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
	log.Printf("[Server] Database initialized at: %s (Retention: %d days)", *dbPath, *retentionDays)

	// 3. Setup Server Handler
	srv := NewServer(db, *token)

	mux := http.NewServeMux()
	mux.HandleFunc("/", srv.ServeDashboard)
	mux.HandleFunc("/api/status", srv.HandleGetStatus)
	mux.HandleFunc("/api/metrics", srv.TokenAuthMiddleware(srv.HandlePostMetrics))

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", *port),
		Handler: mux,
	}

	// 4. Background Metrics Retention Cleaner Daemon
	cleanupTicker := time.NewTicker(1 * time.Hour)
	defer cleanupTicker.Stop()

	// Proactively run an initial cleanup on startup
	if affected, err := CleanupOldMetrics(db, *retentionDays); err != nil {
		log.Printf("[Server] [Warning] Failed to run initial database cleanup: %v", err)
	} else if affected > 0 {
		log.Printf("[Server] Startup cleanup deleted %d expired metric records.", affected)
	}

	go func() {
		log.Println("[Server] Background database retention cleaner daemon started.")
		for range cleanupTicker.C {
			affected, err := CleanupOldMetrics(db, *retentionDays)
			if err != nil {
				log.Printf("[Server] [Error] Background database cleanup failed: %v", err)
			} else if affected > 0 {
				log.Printf("[Server] Background cleanup deleted %d expired metric records.", affected)
			}
		}
	}()

	// 5. Graceful Shutdown & Signal Handlers
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		log.Printf("[Server] Ingestion and Dashboard HTTP server running on port %d...", *port)
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
