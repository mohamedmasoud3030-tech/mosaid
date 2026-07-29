package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
)

type Store struct{ DB *sql.DB }
type Item struct {
	ID              int64 `json:"id"`
	Content, Source string
	Confidence      float64
	Created         string
}

var secret = regexp.MustCompile(`(?i)(api[_ -]?key|token|password|secret)\s*[:=]\s*\S+|sk-[A-Za-z0-9_-]{12,}`)

func (s Store) Candidate(ctx context.Context, content, source string, confidence float64, expires *time.Time) (int64, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return 0, errors.New("empty memory")
	}
	if secret.MatchString(content) {
		return 0, errors.New("secret-like memory denied")
	}
	var exp any
	if expires != nil {
		exp = expires.UTC().Format(time.RFC3339Nano)
	}
	r, e := s.DB.ExecContext(ctx, `INSERT INTO memory_candidates(content,source,confidence,expires_at,state,created_at) VALUES(?,?,?,?,'pending',?)`, content, source, confidence, exp, time.Now().UTC().Format(time.RFC3339Nano))
	if e != nil {
		return 0, e
	}
	return r.LastInsertId()
}
func (s Store) Approve(ctx context.Context, id int64) (int64, error) {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return 0, e
	}
	defer tx.Rollback()
	var c, src string
	var conf float64
	var exp sql.NullString
	e = tx.QueryRowContext(ctx, `SELECT content,source,confidence,expires_at FROM memory_candidates WHERE id=? AND state='pending'`, id).Scan(&c, &src, &conf, &exp)
	if e != nil {
		return 0, e
	}
	r, e := tx.ExecContext(ctx, `INSERT INTO memory_items(content,source,confidence,expires_at,created_at) VALUES(?,?,?,?,?)`, c, src, conf, nullable(exp), time.Now().UTC().Format(time.RFC3339Nano))
	if e != nil {
		return 0, e
	}
	mid, _ := r.LastInsertId()
	if _, e = tx.ExecContext(ctx, `INSERT INTO memory_fts(rowid,content) VALUES(?,?)`, mid, c); e != nil {
		return 0, e
	}
	if _, e = tx.ExecContext(ctx, `UPDATE memory_candidates SET state='approved' WHERE id=?`, id); e != nil {
		return 0, e
	}
	return mid, tx.Commit()
}
func (s Store) Remember(ctx context.Context, content string) (int64, error) {
	id, e := s.Candidate(ctx, content, "telegram:explicit", 1, nil)
	if e != nil {
		return 0, e
	}
	return s.Approve(ctx, id)
}
func (s Store) Search(ctx context.Context, q string, limit int) ([]Item, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, e := s.DB.QueryContext(ctx, `SELECT m.id,m.content,m.source,m.confidence,m.created_at FROM memory_fts f JOIN memory_items m ON m.id=f.rowid WHERE memory_fts MATCH ? AND (m.expires_at IS NULL OR m.expires_at>?) ORDER BY rank LIMIT ?`, q, time.Now().UTC().Format(time.RFC3339Nano), limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		var x Item
		if e = rows.Scan(&x.ID, &x.Content, &x.Source, &x.Confidence, &x.Created); e != nil {
			return nil, e
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
func (s Store) Forget(ctx context.Context, id int64) error {
	tx, e := s.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	_, _ = tx.ExecContext(ctx, `INSERT INTO memory_fts(memory_fts,rowid,content) SELECT 'delete',id,content FROM memory_items WHERE id=?`, id)
	if _, e = tx.ExecContext(ctx, `DELETE FROM memory_links WHERE memory_id=?`, id); e != nil {
		return e
	}
	if _, e = tx.ExecContext(ctx, `DELETE FROM memory_items WHERE id=?`, id); e != nil {
		return e
	}
	return tx.Commit()
}
func (s Store) Export(ctx context.Context) ([]byte, error) {
	rows, e := s.DB.QueryContext(ctx, `SELECT id,content,source,confidence,created_at FROM memory_items ORDER BY id`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var x []Item
	for rows.Next() {
		var i Item
		if e = rows.Scan(&i.ID, &i.Content, &i.Source, &i.Confidence, &i.Created); e != nil {
			return nil, e
		}
		x = append(x, i)
	}
	return json.MarshalIndent(x, "", "  ")
}
func nullable(s sql.NullString) any {
	if s.Valid {
		return s.String
	}
	return nil
}
