package instagram

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const draftColumns = `id,account_id,artifact_id,asset_hash,caption,caption_hash,publish_at,state,container_id,media_id,staging_id,staging_url,staging_expires_at,creation_key,attempts,max_attempts,available_at,authorized_until,last_error,created_at,updated_at`

type Store struct {
	db    *sql.DB
	clock Clock
}

func NewStore(db *sql.DB, clock Clock) *Store {
	if clock == nil {
		clock = RealClock{}
	}
	return &Store{db: db, clock: clock}
}

func (s *Store) Create(ctx context.Context, request PrepareRequest, assetHash, captionHash string) (Draft, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Draft{}, false, err
	}
	defer tx.Rollback()
	existing, err := scanDraft(tx.QueryRowContext(ctx, `SELECT `+draftColumns+` FROM instagram_drafts WHERE creation_key=?`, request.CreationKey))
	if err == nil {
		if existing.AccountID == request.AccountID && existing.ArtifactID == request.ArtifactID && existing.AssetHash == assetHash && existing.CaptionHash == captionHash && existing.PublishAt.Equal(request.PublishAt.UTC()) {
			return existing, false, nil
		}
		return Draft{}, false, ErrDraftConflict
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Draft{}, false, err
	}
	now := s.clock.Now().UTC()
	hash := sha256.Sum256([]byte("instagram-draft:" + request.AccountID + ":" + request.CreationKey))
	id := hex.EncodeToString(hash[:16])
	draft := Draft{
		ID: id, AccountID: request.AccountID, ArtifactID: request.ArtifactID, AssetHash: assetHash, Caption: request.Caption, CaptionHash: captionHash,
		PublishAt: request.PublishAt.UTC(), State: "prepared", CreationKey: request.CreationKey, MaxAttempts: request.MaxAttempts,
		AvailableAt: request.PublishAt.UTC(), CreatedAt: now, UpdatedAt: now,
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO instagram_drafts(id,account_id,artifact_id,asset_hash,caption,caption_hash,publish_at,state,creation_key,max_attempts,available_at,created_at,updated_at) VALUES(?,?,?,?,?,?,?,'prepared',?,?,?,?,?)`,
		draft.ID, draft.AccountID, draft.ArtifactID, draft.AssetHash, draft.Caption, draft.CaptionHash, formatTime(draft.PublishAt), draft.CreationKey, draft.MaxAttempts, formatTime(draft.AvailableAt), formatTime(now), formatTime(now))
	if err != nil {
		return Draft{}, false, err
	}
	if err = insertEvent(ctx, tx, draft.ID, "prepared", "", ""); err != nil {
		return Draft{}, false, err
	}
	return draft, true, tx.Commit()
}

func (s *Store) Get(ctx context.Context, id string) (Draft, error) {
	return scanDraft(s.db.QueryRowContext(ctx, `SELECT `+draftColumns+` FROM instagram_drafts WHERE id=?`, id))
}

func (s *Store) MarkAuthorized(ctx context.Context, id string, expires time.Time) error {
	result, err := s.db.ExecContext(ctx, `UPDATE instagram_drafts SET authorized_until=?,updated_at=? WHERE id=? AND state IN('prepared','failed')`, formatTime(expires), formatTime(s.clock.Now()), id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return ErrApprovalBinding
	}
	return nil
}

func (s *Store) Claim(ctx context.Context, id string) (Draft, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Draft{}, err
	}
	defer tx.Rollback()
	draft, err := scanDraft(tx.QueryRowContext(ctx, `SELECT `+draftColumns+` FROM instagram_drafts WHERE id=?`, id))
	if err != nil {
		return Draft{}, err
	}
	now := s.clock.Now().UTC()
	if draft.State == "published" {
		return draft, nil
	}
	if draft.State != "prepared" && draft.State != "failed" {
		return Draft{}, fmt.Errorf("draft state %s is not claimable", draft.State)
	}
	if draft.Attempts >= draft.MaxAttempts {
		return Draft{}, ErrRetryExhausted
	}
	if draft.AuthorizedUntil == nil || !now.Before(*draft.AuthorizedUntil) {
		return Draft{}, ErrApprovalBinding
	}
	if now.Before(draft.PublishAt) || now.Before(draft.AvailableAt) {
		return Draft{}, ErrNotDue
	}
	result, err := tx.ExecContext(ctx, `UPDATE instagram_drafts SET state='publishing',attempts=attempts+1,updated_at=? WHERE id=? AND state IN('prepared','failed')`, formatTime(now), id)
	if err != nil {
		return Draft{}, err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return Draft{}, errors.New("draft claim race")
	}
	draft.State = "publishing"
	draft.Attempts++
	draft.UpdatedAt = now
	if err = insertEvent(ctx, tx, id, "claimed", "", fmt.Sprintf("attempt=%d", draft.Attempts)); err != nil {
		return Draft{}, err
	}
	return draft, tx.Commit()
}

func (s *Store) SaveStaging(ctx context.Context, id string, media StagedMedia) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := formatTime(s.clock.Now())
	if _, err = tx.ExecContext(ctx, `UPDATE instagram_drafts SET staging_id=?,staging_url=?,staging_expires_at=?,updated_at=? WHERE id=? AND state='publishing'`, media.ID, media.URL, formatTime(media.ExpiresAt), now, id); err != nil {
		return err
	}
	if err = insertEvent(ctx, tx, id, "media_staged", media.ID, ""); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) SaveContainer(ctx context.Context, id, containerID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `UPDATE instagram_drafts SET container_id=?,updated_at=? WHERE id=? AND state='publishing'`, containerID, formatTime(s.clock.Now()), id); err != nil {
		return err
	}
	if err = insertEvent(ctx, tx, id, "container_created", containerID, ""); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Complete(ctx context.Context, id, mediaID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE instagram_drafts SET state='published',media_id=?,last_error='',updated_at=? WHERE id=? AND state='publishing'`, mediaID, formatTime(s.clock.Now()), id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n != 1 {
		return errors.New("draft completion state mismatch")
	}
	if err = insertEvent(ctx, tx, id, "published", mediaID, ""); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Fail(ctx context.Context, id string, cause error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var attempts int
	if err = tx.QueryRowContext(ctx, `SELECT attempts FROM instagram_drafts WHERE id=? AND state='publishing'`, id).Scan(&attempts); err != nil {
		return err
	}
	delay := time.Duration(1<<min(attempts-1, 6)) * time.Second
	now := s.clock.Now().UTC()
	if _, err = tx.ExecContext(ctx, `UPDATE instagram_drafts SET state='failed',available_at=?,last_error=?,updated_at=? WHERE id=? AND state='publishing'`, formatTime(now.Add(delay)), safeError(cause), formatTime(now), id); err != nil {
		return err
	}
	if err = insertEvent(ctx, tx, id, "failed", "", safeError(cause)); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Recover(ctx context.Context) (int64, error) {
	now := formatTime(s.clock.Now())
	result, err := s.db.ExecContext(ctx, `UPDATE instagram_drafts SET state='failed',available_at=?,last_error='recovered interrupted publish',updated_at=? WHERE state='publishing'`, now, now)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) ClearStaging(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE instagram_drafts SET staging_id='',staging_url='',staging_expires_at=NULL,updated_at=? WHERE id=?`, formatTime(s.clock.Now()), id)
	return err
}

func (s *Store) ExpiredStaging(ctx context.Context) ([]Draft, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+draftColumns+` FROM instagram_drafts WHERE staging_id<>'' AND staging_expires_at<=? AND state<>'publishing'`, formatTime(s.clock.Now()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var drafts []Draft
	for rows.Next() {
		draft, err := scanDraft(rows)
		if err != nil {
			return nil, err
		}
		drafts = append(drafts, draft)
	}
	return drafts, rows.Err()
}

func insertEvent(ctx context.Context, tx *sql.Tx, id, kind, providerRef, detail string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO instagram_publish_events(draft_id,kind,provider_ref,detail,created_at) VALUES(?,?,?,?,?)`, id, kind, providerRef, detail, formatTime(time.Now()))
	return err
}

func scanDraft(scanner interface{ Scan(...any) error }) (Draft, error) {
	var draft Draft
	var publishAt, availableAt, createdAt, updatedAt string
	var stagingExpires, authorizedUntil sql.NullString
	err := scanner.Scan(&draft.ID, &draft.AccountID, &draft.ArtifactID, &draft.AssetHash, &draft.Caption, &draft.CaptionHash, &publishAt, &draft.State, &draft.ContainerID, &draft.MediaID, &draft.StagingID, &draft.StagingURL, &stagingExpires, &draft.CreationKey, &draft.Attempts, &draft.MaxAttempts, &availableAt, &authorizedUntil, &draft.LastError, &createdAt, &updatedAt)
	if err != nil {
		return Draft{}, err
	}
	if draft.PublishAt, err = parseTime(publishAt); err != nil {
		return Draft{}, err
	}
	if draft.AvailableAt, err = parseTime(availableAt); err != nil {
		return Draft{}, err
	}
	if draft.CreatedAt, err = parseTime(createdAt); err != nil {
		return Draft{}, err
	}
	if draft.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return Draft{}, err
	}
	if stagingExpires.Valid {
		value, parseErr := parseTime(stagingExpires.String)
		if parseErr != nil {
			return Draft{}, parseErr
		}
		draft.StagingExpiresAt = &value
	}
	if authorizedUntil.Valid {
		value, parseErr := parseTime(authorizedUntil.String)
		if parseErr != nil {
			return Draft{}, parseErr
		}
		draft.AuthorizedUntil = &value
	}
	return draft, nil
}

func formatTime(value time.Time) string         { return value.UTC().Format(time.RFC3339Nano) }
func parseTime(value string) (time.Time, error) { return time.Parse(time.RFC3339Nano, value) }
func hashString(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:])
}
func safeError(err error) string {
	if err == nil {
		return ""
	}
	value := strings.ReplaceAll(err.Error(), "\n", " ")
	if len(value) > 500 {
		value = value[:500]
	}
	return value
}
func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
