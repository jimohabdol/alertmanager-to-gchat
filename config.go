package main

import (
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/BurntSushi/toml"
)

type Config struct {
	Server     ServerConfig     `toml:"server"`
	GoogleChat GoogleChatConfig `toml:"google_chat"`
	Logging    LoggingConfig    `toml:"logging"`
}

type ServerConfig struct {
	ListenAddr string `toml:"listen_addr" env:"LISTEN_ADDR"`
}

type GoogleChatConfig struct {
	WebhookURL string `toml:"webhook_url" env:"GOOGLE_CHAT_WEBHOOK_URL"`
}

type LoggingConfig struct {
	Level string `toml:"level" env:"LOG_LEVEL"`
}

func LoadConfig(path string) (Config, error) {
	var config Config

	config.Server.ListenAddr = ":7000"
	config.Logging.Level = "info"

	if _, err := os.Stat(path); err == nil {
		if _, err := toml.DecodeFile(path, &config); err != nil {
			return config, fmt.Errorf("failed to decode config file: %v", err)
		}
	}

	if v := os.Getenv("LISTEN_ADDR"); v != "" {
		config.Server.ListenAddr = v
	}
	if v := os.Getenv("GOOGLE_CHAT_WEBHOOK_URL"); v != "" {
		config.GoogleChat.WebhookURL = v
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		config.Logging.Level = strings.ToLower(v)
	}

	return config, nil
}

func (c *Config) Validate() error {
	if c.GoogleChat.WebhookURL == "" {
		return fmt.Errorf("Google Chat webhook URL is required")
	}

	if _, err := url.Parse(c.GoogleChat.WebhookURL); err != nil {
		return fmt.Errorf("invalid webhook URL format: %v", err)
	}

	if !strings.HasPrefix(c.GoogleChat.WebhookURL, "https://") {
		return fmt.Errorf("Google Chat webhook URL must use HTTPS")
	}

	if c.Server.ListenAddr == "" {
		return fmt.Errorf("server listen address is required")
	}

	validLogLevels := map[string]bool{
		LogLevelDebug: true,
		LogLevelInfo:  true,
		LogLevelError: true,
	}

	if !validLogLevels[strings.ToLower(c.Logging.Level)] {
		return fmt.Errorf("invalid log level: %s", c.Logging.Level)
	}

	return nil
}
