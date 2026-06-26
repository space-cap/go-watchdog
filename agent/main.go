package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	configPath := flag.String("config", "config.json", "Path to the configuration file")
	flag.Parse()

	log.Println("[Agent] Starting go-watchdog performance monitoring agent...")

	// 1. Load config
	config, err := LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("[Agent] Failed to load config: %v", err)
	}

	log.Printf("[Agent] Configured Host: AgentID=%s, TargetServer=%s, Interval=%ds\n",
		config.AgentID, config.ServerURL, config.IntervalSeconds)

	// 2. Initialize Sender
	sender := NewSender(config.ServerURL, config.AuthToken)

	// 3. Warm-up collection (Calculates baseline CPU tick metrics for subsequent non-blocking reads)
	_, _ = CollectMetrics(config.AgentID)
	time.Sleep(500 * time.Millisecond) // Short pause to stabilize first reading interval

	// 4. Start ticker loop
	ticker := time.NewTicker(time.Duration(config.IntervalSeconds) * time.Second)
	defer ticker.Stop()

	// Capture interrupt and termination signals to exit cleanly
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Println("[Agent] Performance monitoring loop initialized successfully.")

	for {
		select {
		case <-ticker.C:
			// Gather metrics
			metric, err := CollectMetrics(config.AgentID)
			if err != nil {
				log.Printf("[Agent] [Error] Resource collection failed: %v", err)
				continue
			}

			// Send to backend
			if err := sender.Send(metric); err != nil {
				log.Printf("[Agent] [Error] Failed to push metrics: %v", err)
			} else {
				log.Printf("[Agent] Metrics reported. CPU: %.1f%%, RAM: %.1f%%, Disk Partitions: %d",
					metric.CPUPercent, metric.MemPercent, len(metric.Disks))
			}

		case sig := <-sigChan:
			log.Printf("[Agent] Signal %v received. Terminating watchdog agent...", sig)
			return
		}
	}
}
