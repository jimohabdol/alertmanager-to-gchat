package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type Provider interface {
	Send(message *GoogleChatMessage, reqID string) error
}

var sharedHTTPClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		MaxIdleConns:        100,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  true,
		MaxIdleConnsPerHost: 100,
	},
}

type GoogleChatProvider struct {
	WebhookURL string
}

func (g *GoogleChatProvider) Send(message *GoogleChatMessage, reqID string) error {
	timer := prometheus.NewTimer(providerRequestDuration.WithLabelValues("google_chat", "start"))
	defer timer.ObserveDuration()

	payload, err := json.Marshal(message)
	if err != nil {
		providerErrors.WithLabelValues("google_chat").Inc()
		return fmt.Errorf("error marshaling Google Chat message: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, g.WebhookURL, bytes.NewBuffer(payload))
	if err != nil {
		providerErrors.WithLabelValues("google_chat").Inc()
		return fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	resp, err := sharedHTTPClient.Do(req)
	if err != nil {
		providerErrors.WithLabelValues("google_chat").Inc()
		return fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		providerErrors.WithLabelValues("google_chat").Inc()
		return fmt.Errorf("received non-success status code %d: %s", resp.StatusCode, string(bodyBytes))
	}

	alertsSent.WithLabelValues(message.Text).Inc()
	return nil
}
