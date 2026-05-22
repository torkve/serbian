package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Config struct {
	Addr              string   `json:"addr"`
	DBPath            string   `json:"db_path"`
	AudioDir          string   `json:"audio_dir"`
	AuthToken         string   `json:"auth_token"`
	AnthropicAPIKey   string   `json:"anthropic_api_key,omitempty"`
	AnthropicModel    string   `json:"anthropic_model,omitempty"`
	WhisperURL        string   `json:"whisper_url"`
	WhisperLanguage   string   `json:"whisper_language"`
	// PublicURL is the externally-visible base URL (no trailing slash). Used
	// only to print a useful setup link in the startup log when the app is
	// deployed behind a reverse proxy. E.g. "https://example.com/serbian".
	// Leave empty for local dev.
	PublicURL         string   `json:"public_url,omitempty"`
	VAPIDPublic       string   `json:"vapid_public,omitempty"`
	VAPIDPrivate      string   `json:"vapid_private,omitempty"`
	VAPIDSubject      string   `json:"vapid_subject"`
	TimeZone          string   `json:"time_zone"`
	ReminderTimes     []string `json:"reminder_times"`
	DailyClaudeBudget int      `json:"daily_claude_budget"`
}

func defaults() *Config {
	return &Config{
		Addr:              ":8089",
		DBPath:            "data/serbian.db",
		AudioDir:          "data/audio",
		AnthropicModel:    "claude-opus-4-7",
		WhisperURL:        "http://127.0.0.1:8080/inference",
		WhisperLanguage:   "sr",
		VAPIDSubject:      "mailto:user@localhost",
		TimeZone:          "Europe/Belgrade",
		ReminderTimes:     []string{"09:00", "20:00"},
		DailyClaudeBudget: 100,
	}
}

func Load(path string) (*Config, error) {
	cfg := defaults()

	f, err := os.Open(path)
	if err == nil {
		defer f.Close()
		if err := json.NewDecoder(f).Decode(cfg); err != nil {
			return nil, fmt.Errorf("decode config: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return nil, fmt.Errorf("open config: %w", err)
	}

	if v := os.Getenv("SERBIAN_ADDR"); v != "" {
		cfg.Addr = v
	}
	if v := os.Getenv("ANTHROPIC_API_KEY"); v != "" {
		cfg.AnthropicAPIKey = v
	}
	if v := os.Getenv("WHISPER_URL"); v != "" {
		cfg.WhisperURL = v
	}

	if cfg.AuthToken == "" {
		tok, err := RandToken(24)
		if err != nil {
			return nil, err
		}
		cfg.AuthToken = tok
	}

	if err := os.MkdirAll(filepath.Dir(cfg.DBPath), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir db dir: %w", err)
	}
	if err := os.MkdirAll(cfg.AudioDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir audio dir: %w", err)
	}
	return cfg, nil
}

func (c *Config) Save(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(c); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

func RandToken(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
