package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Notifier handles sending alerts to third-party chat systems like Slack and Discord.
type Notifier struct {
	slackWebhookURL   string
	discordWebhookURL string
	client            *http.Client
}

// NewNotifier creates an instance of Notifier.
func NewNotifier(slackURL, discordURL string) *Notifier {
	return &Notifier{
		slackWebhookURL:   slackURL,
		discordWebhookURL: discordURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// SendAlert sends a formatted status transition alert to active notification channels.
func (n *Notifier) SendAlert(targetName, targetURL, status string, latencyMs int, statusCode int, errMsg string) {
	timestamp := time.Now().Format("2006-01-02 15:04:05 KST")
	var msg string

	if status == "OFFLINE" {
		msg = fmt.Sprintf("🚨 *[go-watchdog] 서비스 장애 감지*\n• *서비스명:* %s\n• *상태:* `OFFLINE` (장애)\n• *대상 URL:* %s\n• *에러 메시지:* `%s`\n• *발생 시각:* %s", 
			targetName, targetURL, errMsg, timestamp)
	} else {
		msg = fmt.Sprintf("✅ *[go-watchdog] 서비스 복구 완료*\n• *서비스명:* %s\n• *상태:* `ONLINE` (정상)\n• *대상 URL:* %s\n• *응답 속도:* %dms (HTTP %d)\n• *복구 시각:* %s", 
			targetName, targetURL, latencyMs, statusCode, timestamp)
	}

	if n.slackWebhookURL != "" {
		go n.sendToSlack(msg)
	}

	if n.discordWebhookURL != "" {
		go n.sendToDiscord(msg)
	}
}

func (n *Notifier) sendToSlack(text string) {
	payload := map[string]string{"text": text}
	n.postJSON(n.slackWebhookURL, payload)
}

func (n *Notifier) sendToDiscord(text string) {
	// Discord allows markdown but uses 'content' instead of 'text'
	payload := map[string]string{"content": text}
	n.postJSON(n.discordWebhookURL, payload)
}

func (n *Notifier) postJSON(url string, payload interface{}) {
	data, err := json.Marshal(payload)
	if err != nil {
		return
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return
	}
	defer resp.Body.Close()
}
