package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go-watchdog/common"
)

// Sender handles sending gathered system metrics to the central backend server.
type Sender struct {
	client    *http.Client
	serverURL string
	authToken string
}

// NewSender initializes a new HTTP client metrics sender with a timeout.
func NewSender(serverURL, authToken string) *Sender {
	return &Sender{
		client: &http.Client{
			Timeout: 5 * time.Second,
		},
		serverURL: serverURL,
		authToken: authToken,
	}
}

// Send serializes the Metric DTO to JSON and POSTs it to the backend server.
func (s *Sender) Send(metric *common.Metric) error {
	payload, err := json.Marshal(metric)
	if err != nil {
		return fmt.Errorf("failed to marshal metrics payload: %w", err)
	}

	req, err := http.NewRequest("POST", s.serverURL, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("failed to build HTTP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Agent-Token", s.authToken)

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to execute HTTP POST: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("server responded with status: %s", resp.Status)
	}

	return nil
}
