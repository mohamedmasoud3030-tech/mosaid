package storage

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/message"
)

func openTest(t *testing.T) (*DB, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "m.db")
	d, e := Open(p)
	if e != nil {
		t.Fatal(e)
	}
	return d, p
}
func TestDuplicateUpdate(t *testing.T) {
	d, _ := openTest(t)
	defer d.Close()
	m := message.Inbound{UpdateID: 7, ChatID: 1, UserID: 1, ChatType: "private", Text: "x"}
	ok, e := d.Ingest(context.Background(), m, "h")
	if e != nil || !ok {
		t.Fatal(ok, e)
	}
	ok, e = d.Ingest(context.Background(), m, "h")
	if e != nil || ok {
		t.Fatal("duplicate", ok, e)
	}
}
func TestRestartRecoversRunningInbox(t *testing.T) {
	d, p := openTest(t)
	ctx := context.Background()
	d.Ingest(ctx, message.Inbound{UpdateID: 1}, "h")
	x, e := d.ClaimInbox(ctx, 3)
	if e != nil || x == nil {
		t.Fatal(e)
	}
	d.Close()
	d, e = Open(p)
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	x, e = d.ClaimInbox(ctx, 3)
	if e != nil || x == nil {
		t.Fatal("not recovered", e)
	}
}
func TestOutboxRecoveryAndIdempotency(t *testing.T) {
	d, p := openTest(t)
	ctx := context.Background()
	d.Ingest(ctx, message.Inbound{UpdateID: 2, ChatID: 2}, "h")
	x, _ := d.ClaimInbox(ctx, 3)
	o := message.Outbound{ChatID: 2, Text: "reply"}
	if e := d.CompleteWithOutbox(ctx, x.ID, o, "k"); e != nil {
		t.Fatal(e)
	}
	if e := d.CompleteWithOutbox(ctx, x.ID, o, "k"); e != nil {
		t.Fatal(e)
	}
	ob, e := d.ClaimOutbox(ctx)
	if e != nil || ob == nil {
		t.Fatal(e)
	}
	d.Close()
	d, e = Open(p)
	if e != nil {
		t.Fatal(e)
	}
	defer d.Close()
	ob, e = d.ClaimOutbox(ctx)
	if e != nil || ob == nil || ob.Key != "k" {
		t.Fatal("outbox not recovered", e)
	}
}
func TestSchedulerMigration(t *testing.T) {
	d, _ := openTest(t)
	defer d.Close()
	var applied int
	if err := d.SQL().QueryRow(`SELECT count(*) FROM schema_migrations WHERE version=4`).Scan(&applied); err != nil {
		t.Fatal(err)
	}
	if applied != 1 {
		t.Fatalf("migration 4 count=%d", applied)
	}
	for _, table := range []string{"scheduled_jobs", "scheduled_runs", "job_locks"} {
		var name string
		if err := d.SQL().QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&name); err != nil {
			t.Fatalf("table %s: %v", table, err)
		}
	}
}

func TestPoisonMessageDeadLetters(t *testing.T) {
	d, _ := openTest(t)
	defer d.Close()
	ctx := context.Background()
	d.Ingest(ctx, message.Inbound{UpdateID: 3}, "h")
	x, _ := d.ClaimInbox(ctx, 1)
	if e := d.FailInbox(ctx, x.ID, errors.New("bad"), 1); e != nil {
		t.Fatal(e)
	}
	n, e := d.StateCount(ctx, "inbox_messages", "dead")
	if e != nil || n != 1 {
		t.Fatal(n, e)
	}
}
