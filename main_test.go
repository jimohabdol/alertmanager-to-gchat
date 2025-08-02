package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandleWebhookWithProvider(t *testing.T) {
	tests := []struct {
		name           string
		payload        AlertManagerPayload
		method         string
		contentType    string
		providerFails  bool
		expectedStatus int
	}{
		{
			name: "valid alert",
			payload: AlertManagerPayload{
				Status: "firing",
				Alerts: []Alert{
					{
						Status: "firing",
						Labels: map[string]string{
							"alertname": "TestAlert",
							"severity":  "critical",
						},
						Annotations: map[string]string{
							"description": "Test alert description",
						},
						StartsAt: time.Now(),
					},
				},
			},
			method:         http.MethodPost,
			contentType:    "application/json",
			providerFails:  false,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "invalid method",
			payload:        AlertManagerPayload{},
			method:         http.MethodGet,
			contentType:    "application/json",
			providerFails:  false,
			expectedStatus: http.StatusMethodNotAllowed,
		},
		{
			name: "invalid content type",
			payload: AlertManagerPayload{
				Status: "firing",
				Alerts: []Alert{
					{
						Status: "firing",
						Labels: map[string]string{
							"alertname": "TestAlert",
						},
					},
				},
			},
			method:         http.MethodPost,
			contentType:    "text/plain",
			providerFails:  false,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "provider failure",
			payload: AlertManagerPayload{
				Status: "firing",
				Alerts: []Alert{
					{
						Status: "firing",
						Labels: map[string]string{
							"alertname": "TestAlert",
						},
					},
				},
			},
			method:         http.MethodPost,
			contentType:    "application/json",
			providerFails:  true,
			expectedStatus: http.StatusInternalServerError,
		},
		{
			name: "invalid payload - missing status",
			payload: AlertManagerPayload{
				Alerts: []Alert{
					{
						Status: "firing",
						Labels: map[string]string{
							"alertname": "TestAlert",
						},
					},
				},
			},
			method:         http.MethodPost,
			contentType:    "application/json",
			providerFails:  false,
			expectedStatus: http.StatusBadRequest,
		},
		{
			name: "invalid payload - empty alerts",
			payload: AlertManagerPayload{
				Status: "firing",
				Alerts: []Alert{},
			},
			method:         http.MethodPost,
			contentType:    "application/json",
			providerFails:  false,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProvider := NewMockProvider(tt.providerFails)

			payload, err := json.Marshal(tt.payload)
			if err != nil {
				t.Fatalf("Failed to marshal test payload: %v", err)
			}

			req := httptest.NewRequest(tt.method, "/webhook", bytes.NewBuffer(payload))
			req.Header.Set("Content-Type", tt.contentType)
			w := httptest.NewRecorder()

			logger = NewLogger(LogLevelInfo, nil)

			handleWebhookWithProvider(w, req, mockProvider)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.expectedStatus == http.StatusOK {
				messages := mockProvider.GetSentMessages()
				if len(messages) != 1 {
					t.Errorf("Expected 1 message sent, got %d", len(messages))
				}

				if len(messages) > 0 {
					msg := messages[0].message
					if msg.Text == "" {
						t.Error("Expected non-empty message text")
					}
					if len(msg.Cards) == 0 {
						t.Error("Expected at least one card in the message")
					}
				}
			}
		})
	}
}

func TestConvertToGoogleChatFormat(t *testing.T) {
	tests := []struct {
		name     string
		payload  AlertManagerPayload
		validate func(*testing.T, *GoogleChatMessage)
	}{
		{
			name: "firing alert with all fields",
			payload: AlertManagerPayload{
				Status: "firing",
				Alerts: []Alert{
					{
						Status: "firing",
						Labels: map[string]string{
							"alertname": "HighCPU",
							"severity":  "critical",
						},
						Annotations: map[string]string{
							"description": "CPU usage is high",
							"summary":     "High CPU utilization",
						},
						StartsAt:     time.Now(),
						GeneratorURL: "http://prometheus/graph",
					},
				},
				CommonLabels: map[string]string{
					"team": "platform",
				},
				ExternalURL: "http://alertmanager",
			},
			validate: func(t *testing.T, msg *GoogleChatMessage) {
				if msg.Text == "" {
					t.Error("Expected non-empty message text")
				}
				if len(msg.Cards) == 0 {
					t.Error("Expected at least one card")
				}
				if msg.Cards[0].Header == nil {
					t.Error("Expected card header")
				}
				if !contains(msg.Text, "FIRING") {
					t.Error("Expected FIRING status in message text")
				}
			},
		},
		{
			name: "resolved alert",
			payload: AlertManagerPayload{
				Status: "resolved",
				Alerts: []Alert{
					{
						Status: "resolved",
						Labels: map[string]string{
							"alertname": "HighCPU",
						},
					},
				},
			},
			validate: func(t *testing.T, msg *GoogleChatMessage) {
				if !contains(msg.Text, "RESOLVED") {
					t.Error("Expected RESOLVED status in message text")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := convertToGoogleChatFormat(&tt.payload)
			tt.validate(t, result)
		})
	}
}

func TestValidateAlertPayload(t *testing.T) {
	tests := []struct {
		name    string
		payload AlertManagerPayload
		wantErr bool
	}{
		{
			name: "valid payload",
			payload: AlertManagerPayload{
				Status: "firing",
				Alerts: []Alert{
					{
						Status: "firing",
						Labels: map[string]string{
							"alertname": "TestAlert",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "missing status",
			payload: AlertManagerPayload{
				Alerts: []Alert{
					{
						Status: "firing",
						Labels: map[string]string{
							"alertname": "TestAlert",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "empty alerts",
			payload: AlertManagerPayload{
				Status: "firing",
				Alerts: []Alert{},
			},
			wantErr: true,
		},
		{
			name: "alert without status",
			payload: AlertManagerPayload{
				Status: "firing",
				Alerts: []Alert{
					{
						Labels: map[string]string{
							"alertname": "TestAlert",
						},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "alert without labels",
			payload: AlertManagerPayload{
				Status: "firing",
				Alerts: []Alert{
					{
						Status: "firing",
						Labels: map[string]string{},
					},
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateAlertPayload(&tt.payload)
			if (err != nil) != tt.wantErr {
				t.Errorf("validateAlertPayload() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func contains(s, substr string) bool {
	return bytes.Contains([]byte(s), []byte(substr))
}
