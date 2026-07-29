package memory

import (
	"context"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/storage"
	"path/filepath"
	"testing"
)

func TestMemoryLifecycle(t *testing.T) {
	d, e := storage.Open(filepath.Join(t.TempDir(), "m.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	s := Store{DB: d.SQL()}
	id, e := s.Remember(context.Background(), "prefers concise answers")
	if e != nil {
		t.Fatal(e)
	}
	x, e := s.Search(context.Background(), "concise", 10)
	if e != nil || len(x) != 1 {
		t.Fatal(x, e)
	}
	if e = s.Forget(context.Background(), id); e != nil {
		t.Fatal(e)
	}
	x, _ = s.Search(context.Background(), "concise", 10)
	if len(x) != 0 {
		t.Fatal(x)
	}
}
func TestSecretRejected(t *testing.T) {
	d, _ := storage.Open(filepath.Join(t.TempDir(), "m.db"))
	defer d.Close()
	s := Store{DB: d.SQL()}
	if _, e := s.Remember(context.Background(), "api_key=secret-value"); e == nil {
		t.Fatal("secret stored")
	}
}
