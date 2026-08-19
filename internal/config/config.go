package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/secrets"
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
	BaseURL          string  `json:"base_url"`
	APIKeyFile       string  `json:"api_key_file"`
	Name             string  `json:"name"`
	TimeoutSeconds   int     `json:"timeout_seconds"`
	InputPricePer1M  float64 `json:"input_price_per_1m"`
	OutputPricePer1M float64 `json:"output_price_per_1m"`
}
type Limits struct {
	MaxMessageBytes   int     `json:"max_message_bytes"`
	MaxResponseBytes  int     `json:"max_response_bytes"`
	MaxModelSteps     int     `json:"max_model_steps"`
	MaxToolCalls      int     `json:"max_tool_calls"`
	MaxTokens         int     `json:"max_tokens"`
	MaxCostUSD        float64 `json:"max_cost_usd"`
	MaxRetries        int     `json:"max_retries"`
	MessagesPerMinute int     `json:"messages_per_minute"`
	MessageBurst      int     `json:"message_burst"`
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
	var trailing any
	if err = dec.Decode(&trailing); !errors.Is(err, io.EOF) {
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
	if c.DataDir == "" || !filepath.IsAbs(c.DataDir) {
		return errors.New("data_dir must be absolute")
	}
	c.DataDir = filepath.Clean(c.DataDir)
	if c.Telegram.TokenFile == "" || !filepath.IsAbs(c.Telegram.TokenFile) {
		return errors.New("telegram.token_file must be absolute")
	}
	if c.Model.APIKeyFile == "" || !filepath.IsAbs(c.Model.APIKeyFile) {
		return errors.New("model.api_key_file must be absolute")
	}
	if c.Model.Name == "" || len(c.Model.Name) > 128 {
		return errors.New("model.name required and bounded")
	}
	for _, p := range []float64{c.Model.InputPricePer1M, c.Model.OutputPricePer1M} {
		if p < 0 || p > 1000 || math.IsNaN(p) || math.IsInf(p, 0) {
			return errors.New("model token prices must be between 0 and 1000 USD per 1M tokens")
		}
	}
	u, err := url.Parse(c.Model.BaseURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return errors.New("model.base_url must be HTTPS without credentials or fragment")
	}
	if c.Telegram.PollTimeoutSeconds < 1 || c.Telegram.PollTimeoutSeconds > 60 || c.Model.TimeoutSeconds < 1 || c.Model.TimeoutSeconds > 300 {
		return errors.New("network timeouts must be explicitly configured and bounded")
	}
	if c.Limits.MaxMessageBytes < 1 || c.Limits.MaxMessageBytes > 1024*1024 || c.Limits.MaxResponseBytes < 1 || c.Limits.MaxResponseBytes > 4*1024*1024 {
		return errors.New("message and response limits must be explicitly configured and bounded")
	}
	if c.Limits.MaxModelSteps < 1 || c.Limits.MaxModelSteps > 32 || c.Limits.MaxToolCalls < 1 || c.Limits.MaxToolCalls > 128 || c.Limits.MaxTokens < 1 || c.Limits.MaxTokens > 1000000 {
		return errors.New("model, tool, and token budgets must be explicitly configured and bounded")
	}
	if c.Limits.MaxCostUSD <= 0 || c.Limits.MaxCostUSD > 100 || c.Limits.MaxRetries < 1 || c.Limits.MaxRetries > 20 {
		return errors.New("cost and retry budgets must be explicitly configured and bounded")
	}
	if c.Limits.MessagesPerMinute < 1 || c.Limits.MessagesPerMinute > 600 || c.Limits.MessageBurst < 1 || c.Limits.MessageBurst > c.Limits.MessagesPerMinute {
		return errors.New("Telegram flood limits must be explicitly configured and bounded")
	}
	return nil
}
func ReadSecret(path string) (string, error) {
	value, err := (secrets.FileSource{}).Read(path)
	if err != nil {
		return "", err
	}
	defer value.Destroy()
	return value.String(), nil
}
