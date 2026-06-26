package main

import (
	"encoding/json"
	"os"
)

// Config represents the agent settings loaded from the local configuration file.
type Config struct {
	AgentID         string `json:"agent_id"`         // Unique name of this server instance
	ServerURL       string `json:"server_url"`       // Backend server ingestion endpoint (e.g. http://localhost:9090/api/metrics)
	AuthToken       string `json:"auth_token"`       // Shared security API key/token
	IntervalSeconds int    `json:"interval_seconds"` // Metrics gathering interval in seconds
}

// LoadConfig reads the agent configuration from the specified JSON file.
func LoadConfig(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var config Config
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&config); err != nil {
		return nil, err
	}

	// Apply default interval if invalid or not specified
	if config.IntervalSeconds <= 0 {
		config.IntervalSeconds = 10
	}

	return &config, nil
}
