package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	DataDir         string   `json:"data_dir"`
	OwnerTelegramID int64    `json:"owner_telegram_id"`
	Telegram        Telegram `json:"telegram"`
	Model           Model    `json:"model"`
	Limits          Limits   `json:"limits"`
}
type Telegram struct {
	TokenFile          string `json:"token_file"`
	PollTimeoutSeconds int    `json:"poll_timeout_seconds"`
}
type Model struct {
	BaseURL        string `json:"base_url"`
	APIKeyFile     string `json:"api_key_file"`
	Name           string `json:"name"`
	TimeoutSeconds int    `json:"timeout_seconds"`
}
type Limits struct {
	MaxMessageBytes  int `json:"max_message_bytes"`
	MaxResponseBytes int `json:"max_response_bytes"`
	MaxModelSteps    int `json:"max_model_steps"`
}

func Load(path string) (Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return Config{}, err
	}
	defer f.Close()
	dec := json.NewDecoder(f)
	dec.DisallowUnknownFields()
	var c Config
	if err = dec.Decode(&c); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}
	if dec.More() {
		return Config{}, errors.New("multiple config documents")
	}
	if err = c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}
func (c *Config) Validate() error {
	if c.OwnerTelegramID <= 0 {
		return errors.New("owner_telegram_id must be positive")
	}
	if c.DataDir == "" {
		return errors.New("data_dir required")
	}
	c.DataDir = filepath.Clean(c.DataDir)
	if c.Telegram.TokenFile == "" {
		return errors.New("telegram.token_file required")
	}
	if c.Model.APIKeyFile == "" {
		return errors.New("model.api_key_file required")
	}
	if c.Model.Name == "" {
		return errors.New("model.name required")
	}
	u, err := url.Parse(c.Model.BaseURL)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return errors.New("model.base_url must be HTTPS")
	}
	if c.Telegram.PollTimeoutSeconds <= 0 {
		c.Telegram.PollTimeoutSeconds = 30
	}
	if c.Model.TimeoutSeconds <= 0 {
		c.Model.TimeoutSeconds = 60
	}
	if c.Limits.MaxMessageBytes <= 0 {
		c.Limits.MaxMessageBytes = 16 * 1024
	}
	if c.Limits.MaxResponseBytes <= 0 {
		c.Limits.MaxResponseBytes = 64 * 1024
	}
	if c.Limits.MaxModelSteps <= 0 {
		c.Limits.MaxModelSteps = 4
	}
	return nil
}
func ReadSecret(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("secret file may not be symlink")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("secret file permissions too broad: %o", info.Mode().Perm())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(string(b))
	if s == "" {
		return "", errors.New("empty secret")
	}
	return s, nil
}
