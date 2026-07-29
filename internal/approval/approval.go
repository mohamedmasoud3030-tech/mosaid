package approval

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/audit"
	"time"
)

type Manager struct {
	DB    *sql.DB
	Audit audit.Logger
}
type Request struct {
	ID                       string
	Token                    string
	UserID                   int64
	Tool, ArgsHash, Resource string
	Expires                  time.Time
}

func (m Manager) Create(ctx context.Context, user int64, tool, argsHash, resource string, ttl time.Duration) (Request, error) {
	b := make([]byte, 24)
	if _, e := rand.Read(b); e != nil {
		return Request{}, e
	}
	tok := base64.RawURLEncoding.EncodeToString(b)
	th := sha256.Sum256([]byte(tok))
	idb := sha256.Sum256(append([]byte("id:"), b...))
	r := Request{ID: hex.EncodeToString(idb[:12]), Token: tok, UserID: user, Tool: tool, ArgsHash: argsHash, Resource: resource, Expires: time.Now().UTC().Add(ttl)}
	_, e := m.DB.ExecContext(ctx, `INSERT INTO approval_requests(id,token_hash,user_id,tool_name,args_hash,resource,expires_at,state,created_at) VALUES(?,?,?,?,?,?,?,'pending',?)`, r.ID, hex.EncodeToString(th[:]), user, tool, argsHash, resource, r.Expires.Format(time.RFC3339Nano), time.Now().UTC().Format(time.RFC3339Nano))
	if e == nil {
		_, _ = m.Audit.Append(ctx, audit.Entry{Kind: "approval_requested", UserID: user, Resource: resource, Decision: "pending"})
	}
	return r, e
}
func (m Manager) ResolveToken(ctx context.Context, token string, user int64, decision string) error {
	th := sha256.Sum256([]byte(token))
	var tool, argsHash, resource string
	var storedUser int64
	if err := m.DB.QueryRowContext(ctx, `SELECT user_id,tool_name,args_hash,resource FROM approval_requests WHERE token_hash=?`, hex.EncodeToString(th[:])).Scan(&storedUser, &tool, &argsHash, &resource); err != nil {
		return errors.New("approval not found")
	}
	if storedUser != user {
		return errors.New("approval binding mismatch")
	}
	return m.Resolve(ctx, token, user, tool, argsHash, resource, decision)
}
func (m Manager) Authorize(ctx context.Context, token string, user int64, tool, argsHash, resource string) error {
	th := sha256.Sum256([]byte(token))
	tx, e := m.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var id, t, a, r, state string
	var u int64
	e = tx.QueryRowContext(ctx, `SELECT id,user_id,tool_name,args_hash,resource,state FROM approval_requests WHERE token_hash=?`, hex.EncodeToString(th[:])).Scan(&id, &u, &t, &a, &r, &state)
	if e != nil {
		return errors.New("approval not found")
	}
	if state != "approved" || u != user || t != tool || a != argsHash || r != resource {
		return errors.New("approval not authorized")
	}
	if _, e = tx.ExecContext(ctx, `INSERT INTO approval_uses(approval_id,used_at) VALUES(?,?)`, id, time.Now().UTC().Format(time.RFC3339Nano)); e != nil {
		return errors.New("approval already used")
	}
	return tx.Commit()
}
func (m Manager) Resolve(ctx context.Context, token string, user int64, tool, argsHash, resource, decision string) error {
	if decision != "approved" && decision != "denied" {
		return errors.New("invalid decision")
	}
	th := sha256.Sum256([]byte(token))
	tx, e := m.DB.BeginTx(ctx, nil)
	if e != nil {
		return e
	}
	defer tx.Rollback()
	var id, t, a, r, exp, state string
	var u int64
	e = tx.QueryRowContext(ctx, `SELECT id,user_id,tool_name,args_hash,resource,expires_at,state FROM approval_requests WHERE token_hash=?`, hex.EncodeToString(th[:])).Scan(&id, &u, &t, &a, &r, &exp, &state)
	if e != nil {
		return errors.New("approval not found")
	}
	if state != "pending" {
		return errors.New("approval already resolved")
	}
	et, _ := time.Parse(time.RFC3339Nano, exp)
	if time.Now().UTC().After(et) {
		_, _ = tx.ExecContext(ctx, `UPDATE approval_requests SET state='expired' WHERE id=?`, id)
		_ = tx.Commit()
		return errors.New("approval expired")
	}
	if u != user || t != tool || a != argsHash || r != resource {
		return errors.New("approval binding mismatch")
	}
	res, e := tx.ExecContext(ctx, `UPDATE approval_requests SET state=?,resolved_at=? WHERE id=? AND state='pending'`, decision, time.Now().UTC().Format(time.RFC3339Nano), id)
	if e != nil {
		return e
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return errors.New("approval replay")
	}
	if e = tx.Commit(); e != nil {
		return e
	}
	_, _ = m.Audit.Append(ctx, audit.Entry{Kind: "approval_resolved", UserID: user, Resource: resource, Decision: decision})
	return nil
}
