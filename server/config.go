package main

import (
	"encoding/json"
	"os"
)

// Config represents the server settings loaded from a JSON configuration file.
type Config struct {
	Port          int    `json:"port"`           // Server HTTP bind port (e.g. 9090)
	AuthToken     string `json:"auth_token"`     // Secret API Key required for agents report
	DBPath        string `json:"db_path"`        // SQLite database file path
	RetentionDays int    `json:"retention_days"` // Days to retain metrics in the database
}

// LoadConfig reads the server configuration from the specified JSON file.
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

	// Apply default values if fields are missing or invalid
	if config.Port <= 0 {
		config.Port = 9090
	}
	if config.AuthToken == "" {
		config.AuthToken = "watchdog-secret-token"
	}
	if config.DBPath == "" {
		config.DBPath = "monitoring.db"
	}
	if config.RetentionDays <= 0 {
		config.RetentionDays = 14
	}

	return &config, nil
}
