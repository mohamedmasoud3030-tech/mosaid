package secrets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileSourceAndDestroy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("test-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	value, err := (FileSource{}).Read(path)
	if err != nil || value.String() != "test-value" {
		t.Fatalf("value=%q err=%v", value.String(), err)
	}
	value.Destroy()
	if value.String() != "" {
		t.Fatal("destroy did not clear value")
	}
}

func TestFileSourceRejectsUnsafeFiles(t *testing.T) {
	root := t.TempDir()
	broad := filepath.Join(root, "broad")
	_ = os.WriteFile(broad, []byte("x"), 0o644)
	multi := filepath.Join(root, "multi")
	_ = os.WriteFile(multi, []byte("one\ntwo"), 0o600)
	link := filepath.Join(root, "link")
	_ = os.Symlink(multi, link)
	for _, path := range []string{broad, multi, link} {
		if _, err := (FileSource{}).Read(path); err == nil {
			t.Fatalf("unsafe secret accepted: %s", path)
		}
	}
}
