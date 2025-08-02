package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	configPath     = flag.String("config", "config.toml", "Path to configuration file")
	defaultTimeout = 10 * time.Second
	config         Config
)

const (
	LogLevelDebug = "debug"
	LogLevelInfo  = "info"
	LogLevelError = "error"
)

type Logger struct {
	*log.Logger
	level string
}

func NewLogger(level string, output *os.File) *Logger {
	return &Logger{
		Logger: log.New(output, "", log.LstdFlags),
		level:  level,
	}
}

func (l *Logger) Debug(format string, v ...interface{}) {
	if l.level == LogLevelDebug {
		l.Printf("[DEBUG] "+format, v...)
	}
}

func (l *Logger) Info(format string, v ...interface{}) {
	if l.level == LogLevelDebug || l.level == LogLevelInfo {
		l.Printf("[INFO] "+format, v...)
	}
}

func (l *Logger) Error(format string, v ...interface{}) {
	l.Printf("[ERROR] "+format, v...)
}

var logger *Logger

type AlertManagerPayload struct {
	Receiver          string            `json:"receiver"`
	Status            string            `json:"status"`
	Alerts            []Alert           `json:"alerts"`
	GroupLabels       map[string]string `json:"groupLabels"`
	CommonLabels      map[string]string `json:"commonLabels"`
	CommonAnnotations map[string]string `json:"commonAnnotations"`
	ExternalURL       string            `json:"externalURL"`
}

type Alert struct {
	Status       string            `json:"status"`
	Labels       map[string]string `json:"labels"`
	Annotations  map[string]string `json:"annotations"`
	StartsAt     time.Time         `json:"startsAt"`
	EndsAt       time.Time         `json:"endsAt"`
	GeneratorURL string            `json:"generatorURL"`
	Fingerprint  string            `json:"fingerprint"`
}

type GoogleChatMessage struct {
	Text  string `json:"text,omitempty"`
	Cards []Card `json:"cards,omitempty"`
}

type Card struct {
	Header   *CardHeader   `json:"header,omitempty"`
	Sections []CardSection `json:"sections"`
}

type CardHeader struct {
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
}

type CardSection struct {
	Header  string   `json:"header,omitempty"`
	Widgets []Widget `json:"widgets"`
}

type Widget struct {
	TextParagraph *TextParagraph `json:"textParagraph,omitempty"`
	KeyValue      *KeyValue      `json:"keyValue,omitempty"`
	Buttons       []Button       `json:"buttons,omitempty"`
}

type TextParagraph struct {
	Text string `json:"text"`
}

type KeyValue struct {
	TopLabel         string `json:"topLabel,omitempty"`
	Content          string `json:"content"`
	ContentMultiline bool   `json:"contentMultiline,omitempty"`
	BottomLabel      string `json:"bottomLabel,omitempty"`
	Icon             string `json:"icon,omitempty"`
}

type Button struct {
	TextButton *TextButton `json:"textButton"`
}

type TextButton struct {
	Text    string         `json:"text"`
	OnClick *OnClickAction `json:"onClick"`
}

type OnClickAction struct {
	OpenLink *OpenLink `json:"openLink"`
}

type OpenLink struct {
	URL string `json:"url"`
}

func main() {
	flag.Parse()

	cfg, err := LoadConfig(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	config = cfg

	setupLogger()

	if err := config.Validate(); err != nil {
		logger.Error("Configuration validation failed: %v", err)
		os.Exit(1)
	}

	provider := &GoogleChatProvider{WebhookURL: config.GoogleChat.WebhookURL}

	server := &http.Server{
		Addr:         config.Server.ListenAddr,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", func(w http.ResponseWriter, r *http.Request) {
		handleWebhookWithProvider(w, r, provider)
	})
	mux.HandleFunc("/health", healthCheckHandler)
	mux.Handle("/metrics", promhttp.Handler())

	server.Handler = mux

	go func() {
		logger.Info("Starting AlertManager to Google Chat webhook server on %s", config.Server.ListenAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Server error: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("Server forced to shutdown: %v", err)
	}

	logger.Info("Server exited")
}

func setupLogger() {
	output := os.Stdout

	level := strings.ToLower(config.Logging.Level)
	if level != LogLevelDebug && level != LogLevelInfo && level != LogLevelError {
		level = LogLevelInfo
	}

	logger = NewLogger(level, output)
	logger.Info("Logger initialized with level: %s", level)
}

func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"version":   "1.0.0",
	}

	json.NewEncoder(w).Encode(response)
}

func handleWebhookWithProvider(w http.ResponseWriter, r *http.Request, provider Provider) {
	reqID := fmt.Sprintf("req-%d", time.Now().UnixNano())
	logger.Info("[%s] Received webhook request from %s", reqID, r.RemoteAddr)

	if r.Method != http.MethodPost {
		logger.Error("[%s] Method not allowed: %s", reqID, r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Validation of content type
	if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
		logger.Error("[%s] Invalid content type: %s", reqID, r.Header.Get("Content-Type"))
		http.Error(w, "Content-Type must be application/json", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		logger.Error("[%s] Error reading request body: %v", reqID, err)
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	if len(body) == 0 {
		logger.Error("[%s] Empty request body", reqID)
		http.Error(w, "Empty request body", http.StatusBadRequest)
		return
	}

	logger.Debug("[%s] Received webhook body: %s", reqID, string(body))

	var alertPayload AlertManagerPayload
	if err := json.Unmarshal(body, &alertPayload); err != nil {
		logger.Error("[%s] Error parsing AlertManager payload: %v", reqID, err)
		http.Error(w, "Error parsing AlertManager payload", http.StatusBadRequest)
		return
	}

	// Validate payload
	if err := validateAlertPayload(&alertPayload); err != nil {
		logger.Error("[%s] Invalid alert payload: %v", reqID, err)
		http.Error(w, "Invalid alert payload", http.StatusBadRequest)
		return
	}

	logger.Info("[%s] Received %d alerts with status: %s, alertname: %s",
		reqID,
		len(alertPayload.Alerts),
		alertPayload.Status,
		getAlertName(&alertPayload))

	alertsReceived.WithLabelValues(alertPayload.Status).Inc()

	chatMessage := convertToGoogleChatFormat(&alertPayload)

	logger.Info("[%s] Sending alert to Google Chat", reqID)
	if err := provider.Send(chatMessage, reqID); err != nil {
		logger.Error("[%s] Error sending to Google Chat: %v", reqID, err)
		http.Error(w, "Error sending to Google Chat", http.StatusInternalServerError)
		return
	}

	logger.Info("[%s] Alert processed successfully", reqID)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Alert processed successfully")
}

func validateAlertPayload(payload *AlertManagerPayload) error {
	if payload.Status == "" {
		return fmt.Errorf("status is required")
	}

	if len(payload.Alerts) == 0 {
		return fmt.Errorf("at least one alert is required")
	}

	for i, alert := range payload.Alerts {
		if alert.Status == "" {
			return fmt.Errorf("alert %d status is required", i)
		}
		if len(alert.Labels) == 0 {
			return fmt.Errorf("alert %d must have at least one label", i)
		}
	}

	return nil
}

func convertToGoogleChatFormat(alertPayload *AlertManagerPayload) *GoogleChatMessage {
	message := &GoogleChatMessage{}

	statusText := strings.ToUpper(alertPayload.Status)
	alertName := getAlertName(alertPayload)
	message.Text = fmt.Sprintf("%s Alert: %s (%d alerts)", statusText, alertName, len(alertPayload.Alerts))

	card := Card{
		Header: &CardHeader{
			Title:    fmt.Sprintf("%s Alert: %s", statusText, alertName),
			Subtitle: fmt.Sprintf("%d alert(s)", len(alertPayload.Alerts)),
		},
		Sections: []CardSection{},
	}

	summarySection := createSummarySection(alertPayload)
	card.Sections = append(card.Sections, summarySection)

	for i, alert := range alertPayload.Alerts {
		alertSection := createAlertSection(i+1, alert)
		card.Sections = append(card.Sections, alertSection)
	}

	if alertPayload.ExternalURL != "" {
		externalSection := createExternalURLSection(alertPayload.ExternalURL)
		card.Sections = append(card.Sections, externalSection)
	}

	message.Cards = append(message.Cards, card)
	return message
}

func createSummarySection(alertPayload *AlertManagerPayload) CardSection {
	summarySection := CardSection{
		Header: "Summary",
		Widgets: []Widget{
			{
				KeyValue: &KeyValue{
					TopLabel: "Status",
					Content:  alertPayload.Status,
					Icon:     getStatusIcon(alertPayload.Status),
				},
			},
		},
	}

	if len(alertPayload.CommonLabels) > 0 {
		labelsContent := formatMapAsList(alertPayload.CommonLabels)
		summarySection.Widgets = append(summarySection.Widgets, Widget{
			KeyValue: &KeyValue{
				TopLabel:         "Common Labels",
				Content:          labelsContent,
				ContentMultiline: true,
			},
		})
	}

	if len(alertPayload.CommonAnnotations) > 0 {
		annotationsContent := formatMapAsList(alertPayload.CommonAnnotations)
		summarySection.Widgets = append(summarySection.Widgets, Widget{
			KeyValue: &KeyValue{
				TopLabel:         "Common Annotations",
				Content:          annotationsContent,
				ContentMultiline: true,
			},
		})
	}

	return summarySection
}

func createAlertSection(alertIndex int, alert Alert) CardSection {
	alertSection := CardSection{
		Header:  fmt.Sprintf("Alert #%d", alertIndex),
		Widgets: []Widget{},
	}

	if description, ok := alert.Annotations["description"]; ok {
		alertSection.Widgets = append(alertSection.Widgets, Widget{
			TextParagraph: &TextParagraph{
				Text: description,
			},
		})
	} else if summary, ok := alert.Annotations["summary"]; ok {
		alertSection.Widgets = append(alertSection.Widgets, Widget{
			TextParagraph: &TextParagraph{
				Text: summary,
			},
		})
	}

	if len(alert.Labels) > 0 {
		labelsContent := formatMapAsList(alert.Labels)
		alertSection.Widgets = append(alertSection.Widgets, Widget{
			KeyValue: &KeyValue{
				TopLabel:         "Labels",
				Content:          labelsContent,
				ContentMultiline: true,
			},
		})
	}

	alertSection.Widgets = append(alertSection.Widgets, Widget{
		KeyValue: &KeyValue{
			TopLabel: "Started",
			Content:  alert.StartsAt.Format(time.RFC3339),
		},
	})

	if alert.GeneratorURL != "" {
		alertSection.Widgets = append(alertSection.Widgets, Widget{
			Buttons: []Button{
				{
					TextButton: &TextButton{
						Text: "View in Prometheus",
						OnClick: &OnClickAction{
							OpenLink: &OpenLink{
								URL: alert.GeneratorURL,
							},
						},
					},
				},
			},
		})
	}

	return alertSection
}

func createExternalURLSection(externalURL string) CardSection {
	return CardSection{
		Widgets: []Widget{
			{
				Buttons: []Button{
					{
						TextButton: &TextButton{
							Text: "View in AlertManager",
							OnClick: &OnClickAction{
								OpenLink: &OpenLink{
									URL: externalURL,
								},
							},
						},
					},
				},
			},
		},
	}
}

func formatMapAsList(data map[string]string) string {
	var content strings.Builder
	for k, v := range data {
		content.WriteString(fmt.Sprintf("• %s: %s\n", k, v))
	}
	return content.String()
}

func getAlertName(alertPayload *AlertManagerPayload) string {
	if alertName, ok := alertPayload.CommonLabels["alertname"]; ok {
		return alertName
	}
	if len(alertPayload.Alerts) > 0 {
		if alertName, ok := alertPayload.Alerts[0].Labels["alertname"]; ok {
			return alertName
		}
	}
	return "Unknown Alert"
}

func getStatusIcon(status string) string {
	switch status {
	case "firing":
		return "STAR"
	case "resolved":
		return "EMAIL"
	default:
		return "DESCRIPTION"
	}
}
