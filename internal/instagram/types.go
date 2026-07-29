package instagram

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidDraft    = errors.New("invalid Instagram draft")
	ErrDraftConflict   = errors.New("instagram draft idempotency conflict")
	ErrNotDue          = errors.New("instagram draft is not due")
	ErrApprovalBinding = errors.New("instagram publish approval binding mismatch")
	ErrRetryExhausted  = errors.New("instagram publish retry budget exhausted")
	ErrContainer       = errors.New("instagram media container failed")
)

type PrepareRequest struct {
	AccountID   string    `json:"account_id"`
	ArtifactID  string    `json:"artifact_id"`
	Caption     string    `json:"caption"`
	PublishAt   time.Time `json:"publish_at"`
	CreationKey string    `json:"creation_key"`
	MaxAttempts int       `json:"max_attempts"`
}

type Draft struct {
	ID               string     `json:"id"`
	AccountID        string     `json:"account_id"`
	ArtifactID       string     `json:"artifact_id"`
	AssetHash        string     `json:"asset_hash"`
	Caption          string     `json:"caption"`
	CaptionHash      string     `json:"caption_hash"`
	PublishAt        time.Time  `json:"publish_at"`
	State            string     `json:"state"`
	ContainerID      string     `json:"container_id,omitempty"`
	MediaID          string     `json:"media_id,omitempty"`
	StagingID        string     `json:"staging_id,omitempty"`
	StagingURL       string     `json:"-"`
	StagingExpiresAt *time.Time `json:"staging_expires_at,omitempty"`
	CreationKey      string     `json:"creation_key"`
	Attempts         int        `json:"attempts"`
	MaxAttempts      int        `json:"max_attempts"`
	AvailableAt      time.Time  `json:"available_at"`
	AuthorizedUntil  *time.Time `json:"authorized_until,omitempty"`
	LastError        string     `json:"last_error,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

type PublishBinding struct {
	DraftID     string `json:"draft_id"`
	AccountID   string `json:"account_id"`
	AssetHash   string `json:"asset_hash"`
	CaptionHash string `json:"caption_hash"`
	PublishAt   string `json:"publish_at"`
}

type Preview struct {
	DraftID     string         `json:"draft_id"`
	AccountID   string         `json:"account_id"`
	AssetHash   string         `json:"asset_hash"`
	Caption     string         `json:"caption"`
	CaptionHash string         `json:"caption_hash"`
	PublishAt   time.Time      `json:"publish_at"`
	State       string         `json:"state"`
	Binding     PublishBinding `json:"publish_binding"`
	Warning     string         `json:"warning"`
}

type PublishResult struct {
	DraftID string `json:"draft_id"`
	MediaID string `json:"media_id"`
	State   string `json:"state"`
}

type StagedMedia struct {
	ID        string
	URL       string
	ExpiresAt time.Time
}

type MediaStager interface {
	Stage(context.Context, string, string, []byte, time.Time) (StagedMedia, error)
	Cleanup(context.Context, string) error
}

type ContainerStatus string

const (
	ContainerInProgress ContainerStatus = "IN_PROGRESS"
	ContainerFinished   ContainerStatus = "FINISHED"
	ContainerError      ContainerStatus = "ERROR"
	ContainerExpired    ContainerStatus = "EXPIRED"
)

type MetaClient interface {
	CreateContainer(context.Context, string, string, string, string) (string, error)
	ContainerStatus(context.Context, string) (ContainerStatus, error)
	PublishContainer(context.Context, string, string, string) (string, error)
}

type Clock interface {
	Now() time.Time
}

type Sleeper interface {
	Sleep(context.Context, time.Duration) error
}

type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now().UTC() }

type RealSleeper struct{}

func (RealSleeper) Sleep(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
