package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
)

type Config struct {
	PortalURL           string `json:"portal_url"`
	CheckerToken        string `json:"checker_token"`
	SyncIntervalSeconds int    `json:"sync_interval_seconds"`
}

func main() {
	configPath := flag.String("config", "checker/config.json", "Path to config.json file")
	flag.Parse()

	log.Println("[Go-Checker] Starting watchdog-checker daemon...")

	// Load configuration
	cfg := Config{
		PortalURL:           "http://localhost:3088",
		CheckerToken:        "watchdog-secret-token",
		SyncIntervalSeconds: 30,
	}

	configFile, err := os.Open(*configPath)
	if err == nil {
		if jsonErr := json.NewDecoder(configFile).Decode(&cfg); jsonErr != nil {
			log.Printf("[Go-Checker] [Warning] Config file parse error: %v, using defaults", jsonErr)
		} else {
			log.Printf("[Go-Checker] Config loaded from: %s", *configPath)
		}
		configFile.Close()
	} else {
		log.Printf("[Go-Checker] [Warning] Config file not found at %s, using defaults", *configPath)
	}

	runner := NewCheckerRunner(cfg.PortalURL, cfg.CheckerToken, cfg.SyncIntervalSeconds)
	runner.Start()

	// Wait for OS shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)

	sig := <-sigCh
	log.Printf("[Go-Checker] Signal received (%v). Shutting down...", sig)
	runner.Stop()
	log.Println("[Go-Checker] Terminated cleanly.")
}
