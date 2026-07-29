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
func TestApprovedTokenCannotBeUsedAfterExpiry(t *testing.T) {
	m, d := mgr(t)
	defer d.Close()
	r, err := m.Create(context.Background(), 9, "instagram.publish", "args", "account", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = m.Resolve(context.Background(), r.Token, 9, "instagram.publish", "args", "account", "approved"); err != nil {
		t.Fatal(err)
	}
	if _, err = d.SQL().Exec(`UPDATE approval_requests SET expires_at=? WHERE id=?`, time.Now().UTC().Add(-time.Second).Format(time.RFC3339Nano), r.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = m.AuthorizeReceipt(context.Background(), r.Token, 9, "instagram.publish", "args", "account"); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("err=%v", err)
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
