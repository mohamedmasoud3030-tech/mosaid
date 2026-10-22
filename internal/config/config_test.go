package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadStrict(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.json")
	os.WriteFile(p, []byte(`{"data_dir":"/tmp/m","owner_telegram_id":1,"telegram":{"token_file":"/x","poll_timeout_seconds":30},"model":{"base_url":"https://example.com/v1","api_key_file":"/y","name":"m","timeout_seconds":60},"limits":{"max_message_bytes":16384,"max_response_bytes":65536,"max_model_steps":4,"max_tool_calls":16,"max_tokens":32000,"max_cost_usd":1,"max_retries":5,"messages_per_minute":30,"message_burst":5}}`), 0600)
	if _, e := Load(p); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(p, []byte(`{"unknown":1}`), 0600)
	if _, e := Load(p); e == nil {
		t.Fatal("unknown accepted")
	}
	os.WriteFile(p, []byte(`{"data_dir":"/tmp/m","owner_telegram_id":1,"telegram":{"token_file":"/x","poll_timeout_seconds":30},"model":{"base_url":"https://example.com/v1","api_key_file":"/y","name":"m","timeout_seconds":60},"limits":{}}`), 0600)
	if _, e := Load(p); e == nil {
		t.Fatal("missing security limits accepted")
	}
}
func TestSecretPermissions(t *testing.T) {
	p := filepath.Join(t.TempDir(), "s")
	os.WriteFile(p, []byte("x"), 0644)
	if _, e := ReadSecret(p); e == nil {
		t.Fatal("broad secret accepted")
	}
	os.Chmod(p, 0600)
	if v, e := ReadSecret(p); e != nil || v != "x" {
		t.Fatal(v, e)
	}
}

func TestModelPriceValidation(t *testing.T) {
	base := `{"data_dir":"/tmp/m","owner_telegram_id":1,"telegram":{"token_file":"/x","poll_timeout_seconds":30},"model":{"base_url":"https://example.com/v1","api_key_file":"/y","name":"m","timeout_seconds":60,"input_price_per_1m":%s,"output_price_per_1m":0},"limits":{"max_message_bytes":16384,"max_response_bytes":65536,"max_model_steps":4,"max_tool_calls":16,"max_tokens":32000,"max_cost_usd":1,"max_retries":5,"messages_per_minute":30,"message_burst":5}}`
	p := filepath.Join(t.TempDir(), "c.json")
	for _, price := range []string{"0", "1.5", "1000"} {
		os.WriteFile(p, []byte(fmt.Sprintf(base, price)), 0600)
		if _, e := Load(p); e != nil {
			t.Fatalf("price %s rejected: %v", price, e)
		}
	}
	for _, price := range []string{"-1", "1000.1"} {
		os.WriteFile(p, []byte(fmt.Sprintf(base, price)), 0600)
		if _, e := Load(p); e == nil {
			t.Fatalf("price %s accepted", price)
		}
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "c.json")
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func baseConfigJSON(t *testing.T, mutate func(map[string]any)) string {
	t.Helper()
	document := map[string]any{
		"data_dir":          filepath.Join(t.TempDir(), "data"),
		"owner_telegram_id": 1,
		"telegram":          map[string]any{"token_file": "/x", "poll_timeout_seconds": 30},
		"model":             map[string]any{"base_url": "https://example.com/v1", "api_key_file": "/y", "name": "m", "timeout_seconds": 60},
		"limits": map[string]any{
			"max_message_bytes": 16384, "max_response_bytes": 65536, "max_model_steps": 4,
			"max_tool_calls": 16, "max_tokens": 32000, "max_cost_usd": 1, "max_retries": 5,
			"messages_per_minute": 30, "message_burst": 5,
		},
	}
	mutate(document)
	encoded, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestWorkspaceConfiguration(t *testing.T) {
	workspace := t.TempDir()
	path := writeConfig(t, baseConfigJSON(t, func(d map[string]any) { d["workspace_dir"] = workspace }))
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.WorkspaceDir != filepath.Clean(workspace) {
		t.Fatalf("workspace = %q", cfg.WorkspaceDir)
	}

	relative := writeConfig(t, baseConfigJSON(t, func(d map[string]any) { d["workspace_dir"] = "work" }))
	if _, err := Load(relative); err == nil {
		t.Fatal("relative workspace accepted")
	}
	root := writeConfig(t, baseConfigJSON(t, func(d map[string]any) { d["workspace_dir"] = "/" }))
	if _, err := Load(root); err == nil {
		t.Fatal("filesystem root accepted as workspace")
	}
	missing := writeConfig(t, baseConfigJSON(t, func(d map[string]any) { d["workspace_dir"] = filepath.Join(t.TempDir(), "nope") }))
	if _, err := Load(missing); err == nil {
		t.Fatal("missing workspace accepted")
	}
	file := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(file, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	notDir := writeConfig(t, baseConfigJSON(t, func(d map[string]any) { d["workspace_dir"] = file }))
	if _, err := Load(notDir); err == nil {
		t.Fatal("file accepted as workspace")
	}
	// The data directory holds the database and secret files; it must never be
	// reachable through the workspace.
	parent := t.TempDir()
	dataDir := filepath.Join(parent, "data")
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		t.Fatal(err)
	}
	containing := writeConfig(t, baseConfigJSON(t, func(d map[string]any) {
		d["workspace_dir"] = parent
		d["data_dir"] = dataDir
	}))
	if _, err := Load(containing); err == nil {
		t.Fatal("workspace containing data_dir accepted")
	}
	inside := writeConfig(t, baseConfigJSON(t, func(d map[string]any) {
		d["workspace_dir"] = dataDir
		d["data_dir"] = dataDir
	}))
	if _, err := Load(inside); err == nil {
		t.Fatal("workspace equal to data_dir accepted")
	}
}

func TestAgentSettingsValidation(t *testing.T) {
	for _, accepted := range []map[string]any{
		{},
		{"max_steps": 4, "max_calls_per_step": 4, "max_tool_result_bytes": 8192, "max_tokens": 1024},
		{"max_steps": 1},
	} {
		settings := accepted
		path := writeConfig(t, baseConfigJSON(t, func(d map[string]any) { d["agent"] = settings }))
		if _, err := Load(path); err != nil {
			t.Fatalf("rejected %v: %v", settings, err)
		}
	}
	for _, rejected := range []map[string]any{
		{"max_steps": 0.5},
		{"max_steps": 13},
		{"max_steps": 5},
		{"max_calls_per_step": 9},
		{"max_tool_result_bytes": 100},
		{"max_tokens": 9000},
		{"system_prompt": strings.Repeat("x", 8001)},
	} {
		settings := rejected
		path := writeConfig(t, baseConfigJSON(t, func(d map[string]any) { d["agent"] = settings }))
		if _, err := Load(path); err == nil {
			t.Fatalf("accepted %v", settings)
		}
	}
}

func TestTelegramIsOptionalForTerminalUse(t *testing.T) {
	// The terminal client runs the same agent with no bot at all, so a config
	// with no Telegram section must load.
	path := writeConfig(t, baseConfigJSON(t, func(d map[string]any) { delete(d, "telegram") }))
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("telegram-free config rejected: %v", err)
	}
	if cfg.Telegram.TokenFile != "" {
		t.Fatalf("token file = %q", cfg.Telegram.TokenFile)
	}
	// An absolute token path without a poll timeout is a half-configured daemon.
	halfway := writeConfig(t, baseConfigJSON(t, func(d map[string]any) {
		d["telegram"] = map[string]any{"token_file": "/run/secrets/telegram.token"}
	}))
	if _, err := Load(halfway); err == nil {
		t.Fatal("token without poll timeout accepted")
	}
	// A relative token path stays rejected.
	relative := writeConfig(t, baseConfigJSON(t, func(d map[string]any) {
		d["telegram"] = map[string]any{"token_file": "telegram.token", "poll_timeout_seconds": 30}
	}))
	if _, err := Load(relative); err == nil {
		t.Fatal("relative token path accepted")
	}
}
