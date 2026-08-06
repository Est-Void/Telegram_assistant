package config

import (
	"encoding/json"
	"fmt"
	"os"
)

type Config struct {
	BotToken     string          `json:"bot_token"`
	APIID        int64           `json:"api_id"`
	APIHash      string          `json:"api_hash"`
	TypingSpeed  float64         `json:"typing_speed"`
	AllowedUsers map[string]bool `json:"allowed_users"`
	Userbot      Userbot         `json:"userbot"`
	Ollama       Ollama          `json:"ollama"`
}

type Ollama struct {
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
}

type Userbot struct {
	Phone      string `json:"phone"`
	SessionDir string `json:"session_dir"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	if cfg.Userbot.SessionDir == "" {
		cfg.Userbot.SessionDir = "session"
	}

	if cfg.Ollama.BaseURL == "" {
		cfg.Ollama.BaseURL = "http://localhost:11434"
	}
	if cfg.Ollama.Model == "" {
		cfg.Ollama.Model = "minime:latest"
	}

	return &cfg, nil
}
