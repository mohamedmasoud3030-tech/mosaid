package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/message"
	_ "modernc.org/sqlite"
	"time"
)

type DB struct{ db *sql.DB }

const schema = `
CREATE TABLE IF NOT EXISTS schema_migrations(version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);
INSERT OR IGNORE INTO schema_migrations(version,applied_at) VALUES(1,datetime('now'));
CREATE TABLE IF NOT EXISTS telegram_updates(update_id INTEGER PRIMARY KEY, received_at TEXT NOT NULL, payload_hash TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS inbox_messages(id INTEGER PRIMARY KEY AUTOINCREMENT,update_id INTEGER UNIQUE NOT NULL,message_json BLOB NOT NULL,state TEXT NOT NULL CHECK(state IN('pending','running','completed','failed','dead')),attempts INTEGER NOT NULL DEFAULT 0,last_error TEXT NOT NULL DEFAULT '',available_at TEXT NOT NULL,created_at TEXT NOT NULL,updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS outbox_messages(id INTEGER PRIMARY KEY AUTOINCREMENT,inbox_id INTEGER NOT NULL,idempotency_key TEXT UNIQUE NOT NULL,message_json BLOB NOT NULL,state TEXT NOT NULL CHECK(state IN('pending','sending','sent','dead')),attempts INTEGER NOT NULL DEFAULT 0,last_error TEXT NOT NULL DEFAULT '',available_at TEXT NOT NULL,created_at TEXT NOT NULL,updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS sessions(chat_id INTEGER PRIMARY KEY,updated_at TEXT NOT NULL,summary TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS task_runs(id INTEGER PRIMARY KEY AUTOINCREMENT,inbox_id INTEGER,state TEXT NOT NULL,started_at TEXT,completed_at TEXT,last_error TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS idempotency_keys(key TEXT PRIMARY KEY,created_at TEXT NOT NULL,result_ref TEXT NOT NULL DEFAULT '');
CREATE TABLE IF NOT EXISTS approval_requests(id TEXT PRIMARY KEY,token_hash TEXT UNIQUE NOT NULL,user_id INTEGER NOT NULL,tool_name TEXT NOT NULL,args_hash TEXT NOT NULL,resource TEXT NOT NULL,expires_at TEXT NOT NULL,state TEXT NOT NULL CHECK(state IN('pending','approved','denied','expired')),created_at TEXT NOT NULL,resolved_at TEXT);
CREATE TABLE IF NOT EXISTS audit_entries(seq INTEGER PRIMARY KEY,at TEXT NOT NULL,kind TEXT NOT NULL,user_id INTEGER NOT NULL,resource TEXT NOT NULL,decision TEXT NOT NULL,prev_hash TEXT NOT NULL,entry_hash TEXT UNIQUE NOT NULL,payload_json BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS approval_uses(approval_id TEXT PRIMARY KEY,used_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS session_messages(id INTEGER PRIMARY KEY AUTOINCREMENT,session_id TEXT NOT NULL,role TEXT NOT NULL,content TEXT NOT NULL,created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS session_summaries(session_id TEXT PRIMARY KEY,summary TEXT NOT NULL,updated_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS memory_candidates(id INTEGER PRIMARY KEY AUTOINCREMENT,content TEXT NOT NULL,source TEXT NOT NULL,confidence REAL NOT NULL,expires_at TEXT,state TEXT NOT NULL DEFAULT 'pending',created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS memory_items(id INTEGER PRIMARY KEY AUTOINCREMENT,content TEXT NOT NULL,source TEXT NOT NULL,confidence REAL NOT NULL,expires_at TEXT,created_at TEXT NOT NULL);
CREATE TABLE IF NOT EXISTS memory_links(id INTEGER PRIMARY KEY AUTOINCREMENT,memory_id INTEGER NOT NULL,kind TEXT NOT NULL,target TEXT NOT NULL,FOREIGN KEY(memory_id) REFERENCES memory_items(id));
CREATE VIRTUAL TABLE IF NOT EXISTS memory_fts USING fts5(content, content='memory_items', content_rowid='id');
INSERT OR IGNORE INTO schema_migrations(version,applied_at) VALUES(2,datetime('now'));
INSERT OR IGNORE INTO schema_migrations(version,applied_at) VALUES(3,datetime('now'));
`

func Open(path string) (*DB, error) {
	d, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	d.SetMaxOpenConns(1)
	if _, err = d.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;`); err != nil {
		d.Close()
		return nil, err
	}
	if _, err = d.Exec(schema); err != nil {
		d.Close()
		return nil, err
	}
	if _, err = d.Exec(`UPDATE inbox_messages SET state='pending',updated_at=datetime('now') WHERE state='running'; UPDATE outbox_messages SET state='pending',updated_at=datetime('now') WHERE state='sending';`); err != nil {
		d.Close()
		return nil, err
	}
	return &DB{d}, nil
}
func (d *DB) Close() error { return d.db.Close() }
func (d *DB) SQL() *sql.DB { return d.db }
func now() string          { return time.Now().UTC().Format(time.RFC3339Nano) }
func (d *DB) Ingest(ctx context.Context, m message.Inbound, hash string) (bool, error) {
	b, _ := json.Marshal(m)
	tx, e := d.db.BeginTx(ctx, nil)
	if e != nil {
		return false, e
	}
	defer tx.Rollback()
	r, e := tx.ExecContext(ctx, `INSERT OR IGNORE INTO telegram_updates(update_id,received_at,payload_hash) VALUES(?,?,?)`, m.UpdateID, now(), hash)
	if e != nil {
		return false, e
	}
	n, _ := r.RowsAffected()
	if n == 0 {
		return false, nil
	}
	t := now()
	_, e = tx.ExecContext(ctx, `INSERT INTO inbox_messages(update_id,message_json,state,available_at,created_at,updated_at) VALUES(?,?,'pending',?,?,?)`, m.UpdateID, b, t, t, t)
	if e != nil {
		return false, e
	}
	return true, tx.Commit()
}

type Inbox struct {
	ID       int64
	Message  message.Inbound
	Attempts int
}

func (d *DB) ClaimInbox(ctx context.Context, max int) (*Inbox, error) {
	tx, e := d.db.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	var x Inbox
	var b []byte
	e = tx.QueryRowContext(ctx, `SELECT id,message_json,attempts FROM inbox_messages WHERE state='pending' AND available_at<=? ORDER BY id LIMIT 1`, now()).Scan(&x.ID, &b, &x.Attempts)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	r, e := tx.ExecContext(ctx, `UPDATE inbox_messages SET state='running',attempts=attempts+1,updated_at=? WHERE id=? AND state='pending'`, now(), x.ID)
	if e != nil {
		return nil, e
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return nil, nil
	}
	if e = json.Unmarshal(b, &x.Message); e != nil {
		return nil, e
	}
	return &x, tx.Commit()
}
func (d *DB) CompleteWithOutbox(ctx context.Context, inbox int64, out message.Outbound, key string) error {
	b, _ := json.Marshal(out)
	tx, e := d.db.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	t := now()
	if _, e = tx.ExecContext(ctx, `INSERT OR IGNORE INTO outbox_messages(inbox_id,idempotency_key,message_json,state,available_at,created_at,updated_at) VALUES(?,?,?,'pending',?,?,?)`, inbox, key, b, t, t, t); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE inbox_messages SET state='completed',updated_at=? WHERE id=?`, t, inbox); e != nil {
		return e
	}
	return tx.Commit()
}
func (d *DB) FailInbox(ctx context.Context, id int64, err error, max int) error {
	state := "pending"
	var attempts int
	_ = d.db.QueryRowContext(ctx, `SELECT attempts FROM inbox_messages WHERE id=?`, id).Scan(&attempts)
	if attempts >= max {
		state = "dead"
	}
	delay := time.Duration(1<<min(attempts, 6)) * time.Second
	_, e := d.db.ExecContext(ctx, `UPDATE inbox_messages SET state=?,last_error=?,available_at=?,updated_at=? WHERE id=?`, state, safeErr(err), time.Now().UTC().Add(delay).Format(time.RFC3339Nano), now(), id)
	return e
}

type Outbox struct {
	ID       int64
	Message  message.Outbound
	Attempts int
	Key      string
}

func (d *DB) ClaimOutbox(ctx context.Context) (*Outbox, error) {
	tx, e := d.db.BeginTx(ctx, nil)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback()
	var x Outbox
	var b []byte
	e = tx.QueryRowContext(ctx, `SELECT id,message_json,attempts,idempotency_key FROM outbox_messages WHERE state='pending' AND available_at<=? ORDER BY id LIMIT 1`, now()).Scan(&x.ID, &b, &x.Attempts, &x.Key)
	if errors.Is(e, sql.ErrNoRows) {
		return nil, nil
	}
	if e != nil {
		return nil, e
	}
	r, e := tx.ExecContext(ctx, `UPDATE outbox_messages SET state='sending',attempts=attempts+1,updated_at=? WHERE id=? AND state='pending'`, now(), x.ID)
	if e != nil {
		return nil, e
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return nil, nil
	}
	if e = json.Unmarshal(b, &x.Message); e != nil {
		return nil, e
	}
	return &x, tx.Commit()
}
func (d *DB) OutboxSent(ctx context.Context, id int64) error {
	_, e := d.db.ExecContext(ctx, `UPDATE outbox_messages SET state='sent',updated_at=? WHERE id=?`, now(), id)
	return e
}
func (d *DB) OutboxFailed(ctx context.Context, id int64, err error, max int) error {
	var a int
	_ = d.db.QueryRowContext(ctx, `SELECT attempts FROM outbox_messages WHERE id=?`, id).Scan(&a)
	state := "pending"
	if a >= max {
		state = "dead"
	}
	delay := time.Duration(1<<min(a, 6)) * time.Second
	_, e := d.db.ExecContext(ctx, `UPDATE outbox_messages SET state=?,last_error=?,available_at=?,updated_at=? WHERE id=?`, state, safeErr(err), time.Now().UTC().Add(delay).Format(time.RFC3339Nano), now(), id)
	return e
}
func (d *DB) StateCount(ctx context.Context, table, state string) (int, error) {
	if table != "inbox_messages" && table != "outbox_messages" {
		return 0, fmt.Errorf("bad table")
	}
	var n int
	e := d.db.QueryRowContext(ctx, `SELECT count(*) FROM `+table+` WHERE state=?`, state).Scan(&n)
	return n, e
}
func safeErr(e error) string {
	if e == nil {
		return ""
	}
	s := e.Error()
	if len(s) > 500 {
		s = s[:500]
	}
	return s
}
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
