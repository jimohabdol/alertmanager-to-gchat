package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server     ServerConfig     `toml:"server"`
	GoogleChat GoogleChatConfig `toml:"google_chat"`
	Logging    LoggingConfig    `toml:"logging"`
}

type ServerConfig struct {
	ListenAddr string `toml:"listen_addr"`
}

type GoogleChatConfig struct {
	WebhookURL string `toml:"webhook_url"`
}

type LoggingConfig struct {
	Level string `toml:"level"`
}

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

	if err := loadConfig(*configPath); err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	setupLogger()

	if config.GoogleChat.WebhookURL == "" {
		logger.Error("Google Chat webhook URL is required in config")
		os.Exit(1)
	}

	http.HandleFunc("/webhook", handleWebhook)
	http.HandleFunc("/health", healthCheckHandler)

	logger.Info("Starting AlertManager to Google Chat webhook server on %s", config.Server.ListenAddr)
	logger.Error("Server terminated: %v", http.ListenAndServe(config.Server.ListenAddr, nil))
}

func loadConfig(path string) error {
	config = Config{
		Server: ServerConfig{
			ListenAddr: ":7000",
		},
		Logging: LoggingConfig{
			Level: LogLevelInfo,
		},
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %v", err)
	}

	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return fmt.Errorf("config file not found at %s", absPath)
	}

	if _, err := toml.DecodeFile(absPath, &config); err != nil {
		return fmt.Errorf("failed to decode config file: %v", err)
	}

	return nil
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
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "OK")
}

func handleWebhook(w http.ResponseWriter, r *http.Request) {
	reqID := fmt.Sprintf("req-%d", time.Now().UnixNano())
	logger.Info("[%s] Received webhook request from %s", reqID, r.RemoteAddr)

	if r.Method != http.MethodPost {
		logger.Error("[%s] Method not allowed: %s", reqID, r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		logger.Error("[%s] Error reading request body: %v", reqID, err)
		http.Error(w, "Error reading request body", http.StatusInternalServerError)
		return
	}
	defer r.Body.Close()

	logger.Debug("[%s] Received webhook body: %s", reqID, string(body))

	var alertPayload AlertManagerPayload
	if err := json.Unmarshal(body, &alertPayload); err != nil {
		logger.Error("[%s] Error parsing AlertManager payload: %v", reqID, err)
		http.Error(w, "Error parsing AlertManager payload", http.StatusBadRequest)
		return
	}

	logger.Info("[%s] Received %d alerts with status: %s, alertname: %s",
		reqID,
		len(alertPayload.Alerts),
		alertPayload.Status,
		getAlertName(&alertPayload))

	chatMessage := convertToGoogleChatFormat(&alertPayload)

	logger.Info("[%s] Sending alert to Google Chat", reqID)
	if err := sendToGoogleChat(chatMessage, reqID); err != nil {
		logger.Error("[%s] Error sending to Google Chat: %v", reqID, err)
		http.Error(w, "Error sending to Google Chat", http.StatusInternalServerError)
		return
	}

	logger.Info("[%s] Alert processed successfully", reqID)
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Alert processed successfully")
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
		var labelsContent strings.Builder
		for k, v := range alertPayload.CommonLabels {
			labelsContent.WriteString(fmt.Sprintf("• %s: %s\n", k, v))
		}

		summarySection.Widgets = append(summarySection.Widgets, Widget{
			KeyValue: &KeyValue{
				TopLabel:         "Common Labels",
				Content:          labelsContent.String(),
				ContentMultiline: true,
			},
		})
	}

	if len(alertPayload.CommonAnnotations) > 0 {
		var annotationsContent strings.Builder
		for k, v := range alertPayload.CommonAnnotations {
			annotationsContent.WriteString(fmt.Sprintf("• %s: %s\n", k, v))
		}

		summarySection.Widgets = append(summarySection.Widgets, Widget{
			KeyValue: &KeyValue{
				TopLabel:         "Common Annotations",
				Content:          annotationsContent.String(),
				ContentMultiline: true,
			},
		})
	}

	card.Sections = append(card.Sections, summarySection)

	for i, alert := range alertPayload.Alerts {
		alertSection := CardSection{
			Header:  fmt.Sprintf("Alert #%d", i+1),
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

		var detailsContent strings.Builder
		for k, v := range alert.Labels {
			detailsContent.WriteString(fmt.Sprintf("• %s: %s\n", k, v))
		}

		alertSection.Widgets = append(alertSection.Widgets, Widget{
			KeyValue: &KeyValue{
				TopLabel:         "Labels",
				Content:          detailsContent.String(),
				ContentMultiline: true,
			},
		})

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

		card.Sections = append(card.Sections, alertSection)
	}

	if alertPayload.ExternalURL != "" {
		card.Sections = append(card.Sections, CardSection{
			Widgets: []Widget{
				{
					Buttons: []Button{
						{
							TextButton: &TextButton{
								Text: "View in AlertManager",
								OnClick: &OnClickAction{
									OpenLink: &OpenLink{
										URL: alertPayload.ExternalURL,
									},
								},
							},
						},
					},
				},
			},
		})
	}

	message.Cards = append(message.Cards, card)
	return message
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
		return "CHECK"
	default:
		return "DESCRIPTION"
	}
}

func sendToGoogleChat(message *GoogleChatMessage, reqID string) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("error marshaling Google Chat message: %v", err)
	}

	logger.Debug("[%s] Google Chat payload: %s", reqID, string(payload))

	client := http.Client{Timeout: defaultTimeout}
	req, err := http.NewRequest(http.MethodPost, config.GoogleChat.WebhookURL, bytes.NewBuffer(payload))
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("Content-Type", "application/json")
	startTime := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(startTime)

	if err != nil {
		return fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		logger.Error("[%s] Google Chat API returned error status: %d, response: %s", reqID, resp.StatusCode, string(bodyBytes))
		return fmt.Errorf("received non-success status code %d: %s", resp.StatusCode, string(bodyBytes))
	}

	logger.Info("[%s] Successfully sent message to Google Chat (status: %d, elapsed: %s)", reqID, resp.StatusCode, elapsed)
	return nil
}
