package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadStrict(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.json")
	os.WriteFile(p, []byte(`{"data_dir":"/tmp/m","owner_telegram_id":1,"telegram":{"token_file":"/x"},"model":{"base_url":"https://example.com/v1","api_key_file":"/y","name":"m"},"limits":{}}`), 0600)
	if _, e := Load(p); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(p, []byte(`{"unknown":1}`), 0600)
	if _, e := Load(p); e == nil {
		t.Fatal("unknown accepted")
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
