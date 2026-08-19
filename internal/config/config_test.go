package config

import (
	"fmt"
	"os"
	"path/filepath"
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
