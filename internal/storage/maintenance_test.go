package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/message"
)

func TestIntegrityBackupAndRestore(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	db, err := Open(filepath.Join(root, "live.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Ingest(ctx, message.Inbound{UpdateID: 77, Text: "durable"}, "hash"); err != nil {
		t.Fatal(err)
	}
	if err = db.IntegrityCheck(ctx); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(root, "backup.db")
	if err = db.Backup(ctx, backup); err != nil {
		t.Fatal(err)
	}
	if err = VerifyBackup(ctx, backup); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	restored := filepath.Join(root, "restored.db")
	if err = RestoreBackup(ctx, backup, restored); err != nil {
		t.Fatal(err)
	}
	restoredDB, err := Open(restored)
	if err != nil {
		t.Fatal(err)
	}
	defer restoredDB.Close()
	var count int
	if err = restoredDB.SQL().QueryRow(`SELECT count(*) FROM telegram_updates WHERE update_id=77`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	if err = RestoreBackup(ctx, backup, restored); err == nil {
		t.Fatal("restore overwrote existing database")
	}
}

func TestCorruptBackupRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "corrupt.db")
	if err := os.WriteFile(path, []byte("not sqlite"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyBackup(context.Background(), path); err == nil {
		t.Fatal("corrupt backup accepted")
	}
}

func TestFailedMigrationRollsBack(t *testing.T) {
	db, _ := openTest(t)
	defer db.Close()
	err := applyMigration(context.Background(), db.SQL(), 99, `CREATE TABLE should_rollback(id INTEGER); THIS IS INVALID SQL;`)
	if err == nil {
		t.Fatal("invalid migration succeeded")
	}
	var count int
	if err = db.SQL().QueryRow(`SELECT count(*) FROM schema_migrations WHERE version=99`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("migration recorded count=%d err=%v", count, err)
	}
	if err = db.SQL().QueryRow(`SELECT count(*) FROM sqlite_master WHERE type='table' AND name='should_rollback'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial table count=%d err=%v", count, err)
	}
}
