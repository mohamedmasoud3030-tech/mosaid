package audit

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strconv"
	"time"
)

type Logger struct{ DB *sql.DB }
type Entry struct {
	Seq      int64  `json:"seq"`
	At       string `json:"at"`
	Kind     string `json:"kind"`
	UserID   int64  `json:"user_id"`
	Resource string `json:"resource"`
	Decision string `json:"decision"`
	PrevHash string `json:"prev_hash"`
	Hash     string `json:"hash"`
}

func (l Logger) Append(ctx context.Context, e Entry) (Entry, error) {
	tx, err := l.DB.BeginTx(ctx, nil)
	if err != nil {
		return e, err
	}
	defer tx.Rollback()
	var seq int64
	var prev string
	err = tx.QueryRowContext(ctx, `SELECT seq,entry_hash FROM audit_entries ORDER BY seq DESC LIMIT 1`).Scan(&seq, &prev)
	if errors.Is(err, sql.ErrNoRows) {
		seq = 0
		prev = ""
	} else if err != nil {
		return e, err
	}
	e.Seq = seq + 1
	e.At = time.Now().UTC().Format(time.RFC3339Nano)
	e.PrevHash = prev
	payload, _ := json.Marshal(struct {
		Seq                      int64
		At, Kind                 string
		UserID                   int64
		Resource, Decision, Prev string
	}{e.Seq, e.At, e.Kind, e.UserID, e.Resource, e.Decision, e.PrevHash})
	h := sha256.Sum256(payload)
	e.Hash = hex.EncodeToString(h[:])
	_, err = tx.ExecContext(ctx, `INSERT INTO audit_entries(seq,at,kind,user_id,resource,decision,prev_hash,entry_hash,payload_json) VALUES(?,?,?,?,?,?,?,?,?)`, e.Seq, e.At, e.Kind, e.UserID, e.Resource, e.Decision, e.PrevHash, e.Hash, payload)
	if err != nil {
		return e, err
	}
	return e, tx.Commit()
}
func (l Logger) Verify(ctx context.Context) error {
	rows, err := l.DB.QueryContext(ctx, `SELECT seq,at,kind,user_id,resource,decision,prev_hash,entry_hash FROM audit_entries ORDER BY seq`)
	if err != nil {
		return err
	}
	defer rows.Close()
	prev := ""
	var expected int64 = 1
	for rows.Next() {
		var e Entry
		if rows.Scan(&e.Seq, &e.At, &e.Kind, &e.UserID, &e.Resource, &e.Decision, &e.PrevHash, &e.Hash) != nil {
			return errors.New("scan")
		}
		if e.Seq != expected || e.PrevHash != prev {
			return errors.New("audit chain broken at " + strconv.FormatInt(e.Seq, 10))
		}
		payload, _ := json.Marshal(struct {
			Seq                      int64
			At, Kind                 string
			UserID                   int64
			Resource, Decision, Prev string
		}{e.Seq, e.At, e.Kind, e.UserID, e.Resource, e.Decision, e.PrevHash})
		h := sha256.Sum256(payload)
		if hex.EncodeToString(h[:]) != e.Hash {
			return errors.New("audit hash mismatch")
		}
		prev = e.Hash
		expected++
	}
	return rows.Err()
}
