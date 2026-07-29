package approval

import (
	"context"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/audit"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/storage"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mgr(t *testing.T) (Manager, *storage.DB) {
	d, e := storage.Open(filepath.Join(t.TempDir(), "d.db"))
	if e != nil {
		t.Fatal(e)
	}
	return Manager{DB: d.SQL(), Audit: audit.Logger{DB: d.SQL()}}, d
}
func TestBoundAndSingleUse(t *testing.T) {
	m, d := mgr(t)
	defer d.Close()
	r, e := m.Create(context.Background(), 1, "workspace.write", "abc", "f", time.Minute)
	if e != nil {
		t.Fatal(e)
	}
	if e = m.Resolve(context.Background(), r.Token, 1, "workspace.write", "changed", "f", "approved"); e == nil || !strings.Contains(e.Error(), "binding") {
		t.Fatal(e)
	}
	if e = m.Resolve(context.Background(), r.Token, 1, "workspace.write", "abc", "f", "approved"); e != nil {
		t.Fatal(e)
	}
	if e = m.Resolve(context.Background(), r.Token, 1, "workspace.write", "abc", "f", "approved"); e == nil {
		t.Fatal("replay")
	}
	if e = m.Audit.Verify(context.Background()); e != nil {
		t.Fatal(e)
	}
}
func TestExpired(t *testing.T) {
	m, d := mgr(t)
	defer d.Close()
	r, e := m.Create(context.Background(), 1, "x", "a", "r", -time.Second)
	if e != nil {
		t.Fatal(e)
	}
	if e = m.Resolve(context.Background(), r.Token, 1, "x", "a", "r", "approved"); e == nil {
		t.Fatal("expired accepted")
	}
}
