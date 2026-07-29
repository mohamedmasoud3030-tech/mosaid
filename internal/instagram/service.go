package instagram

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/approval"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/audit"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/images"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/policy"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/skills"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/tools"
)

var accountPattern = regexp.MustCompile(`^[0-9]{1,64}$`)

type Service struct {
	Store        *Store
	Artifacts    *images.Store
	Meta         MetaClient
	Stager       MediaStager
	Approvals    *approval.Manager
	Audit        *audit.Logger
	Clock        Clock
	Sleeper      Sleeper
	OwnerID      int64
	PollInterval time.Duration
	MaxPolls     int
}

func (s *Service) Prepare(ctx context.Context, request PrepareRequest) (Draft, error) {
	if err := s.validateDependencies(); err != nil {
		return Draft{}, err
	}
	now := s.Clock.Now().UTC()
	if !accountPattern.MatchString(request.AccountID) || len(request.ArtifactID) != 64 || strings.TrimSpace(request.Caption) != request.Caption || request.Caption == "" || utf8.RuneCountInString(request.Caption) > 2200 || strings.ContainsRune(request.Caption, 0) {
		return Draft{}, ErrInvalidDraft
	}
	if request.PublishAt.IsZero() || request.PublishAt.UTC().Before(now.Add(-time.Minute)) || request.PublishAt.UTC().After(now.Add(75*24*time.Hour)) {
		return Draft{}, ErrInvalidDraft
	}
	if request.CreationKey == "" || len(request.CreationKey) > 256 || strings.ContainsRune(request.CreationKey, 0) {
		return Draft{}, ErrInvalidDraft
	}
	if request.MaxAttempts == 0 {
		request.MaxAttempts = 3
	}
	if request.MaxAttempts < 1 || request.MaxAttempts > 10 {
		return Draft{}, ErrInvalidDraft
	}
	artifact, err := s.Artifacts.Read(request.ArtifactID)
	if err != nil || artifact.SHA256 != request.ArtifactID {
		return Draft{}, ErrInvalidDraft
	}
	captionHash := hashString(request.Caption)
	draft, created, err := s.Store.Create(ctx, request, artifact.SHA256, captionHash)
	if err != nil {
		return Draft{}, err
	}
	if created {
		if err = s.record(ctx, "instagram_prepare", draft.AccountID, "prepared"); err != nil {
			return Draft{}, err
		}
	}
	return draft, nil
}

func (s *Service) Preview(ctx context.Context, id string) (Preview, error) {
	draft, err := s.Store.Get(ctx, id)
	if err != nil {
		return Preview{}, err
	}
	binding := bindingFor(draft)
	return Preview{
		DraftID: draft.ID, AccountID: draft.AccountID, AssetHash: draft.AssetHash, Caption: draft.Caption, CaptionHash: draft.CaptionHash,
		PublishAt: draft.PublishAt, State: draft.State, Binding: binding,
		Warning: "Preview only. This does not authorize or perform publishing.",
	}, nil
}

func (s *Service) RequestApproval(ctx context.Context, id string, userID int64, ttl time.Duration) (approval.Request, json.RawMessage, error) {
	if s.Approvals == nil || ttl <= 0 || ttl > 24*time.Hour {
		return approval.Request{}, nil, ErrApprovalBinding
	}
	draft, err := s.Store.Get(ctx, id)
	if err != nil {
		return approval.Request{}, nil, err
	}
	binding := bindingFor(draft)
	arguments, err := canonicalBinding(binding)
	if err != nil {
		return approval.Request{}, nil, err
	}
	hash := sha256.Sum256(arguments)
	request, err := s.Approvals.Create(ctx, userID, "instagram.publish", hex.EncodeToString(hash[:]), draft.AccountID, ttl)
	return request, arguments, err
}

func (s *Service) PublishWithToken(ctx context.Context, id string, userID int64, token string) (PublishResult, error) {
	if s.Approvals == nil {
		return PublishResult{}, ErrApprovalBinding
	}
	draft, err := s.Store.Get(ctx, id)
	if err != nil {
		return PublishResult{}, err
	}
	binding := bindingFor(draft)
	arguments, _ := canonicalBinding(binding)
	hash := sha256.Sum256(arguments)
	receipt, err := s.Approvals.AuthorizeReceipt(ctx, token, userID, "instagram.publish", hex.EncodeToString(hash[:]), draft.AccountID)
	if err != nil {
		return PublishResult{}, ErrApprovalBinding
	}
	return s.PublishApproved(ctx, binding, receipt)
}

func (s *Service) PublishApproved(ctx context.Context, binding PublishBinding, receipt approval.Receipt) (PublishResult, error) {
	if err := s.validateDependencies(); err != nil {
		return PublishResult{}, err
	}
	draft, err := s.Store.Get(ctx, binding.DraftID)
	if err != nil {
		return PublishResult{}, err
	}
	expected := bindingFor(draft)
	arguments, _ := canonicalBinding(binding)
	hash := sha256.Sum256(arguments)
	if binding != expected || receipt.UserID != s.OwnerID || receipt.Tool != "instagram.publish" || receipt.Resource != draft.AccountID || receipt.ArgsHash != hex.EncodeToString(hash[:]) || !s.Clock.Now().UTC().Before(receipt.Expires) {
		return PublishResult{}, ErrApprovalBinding
	}
	if err = s.Store.MarkAuthorized(ctx, draft.ID, receipt.Expires); err != nil {
		return PublishResult{}, err
	}
	if err = s.record(ctx, "instagram_publish_approval", draft.AccountID, "authorized"); err != nil {
		return PublishResult{}, err
	}
	return s.Resume(ctx, draft.ID)
}

func (s *Service) Resume(ctx context.Context, id string) (result PublishResult, err error) {
	if err = s.validateDependencies(); err != nil {
		return PublishResult{}, err
	}
	draft, err := s.Store.Get(ctx, id)
	if err != nil {
		return PublishResult{}, err
	}
	if draft.State == "published" {
		return PublishResult{DraftID: draft.ID, MediaID: draft.MediaID, State: draft.State}, nil
	}
	draft, err = s.Store.Claim(ctx, id)
	if err != nil {
		return PublishResult{}, err
	}
	failed := true
	defer func() {
		if failed && err != nil {
			_ = s.Store.Fail(context.WithoutCancel(ctx), draft.ID, err)
			_ = s.record(context.WithoutCancel(ctx), "instagram_publish", draft.AccountID, "failed")
		}
	}()

	if draft.StagingID == "" || draft.StagingExpiresAt == nil || !s.Clock.Now().UTC().Before(*draft.StagingExpiresAt) {
		artifact, readErr := s.Artifacts.Read(draft.ArtifactID)
		if readErr != nil {
			err = readErr
			return PublishResult{}, err
		}
		staged, stageErr := s.Stager.Stage(ctx, artifact.ID, artifact.MIME, artifact.Data, s.Clock.Now().UTC().Add(time.Hour))
		if stageErr != nil || !validStagedMedia(staged, s.Clock.Now()) {
			if stageErr != nil {
				err = stageErr
			} else {
				err = errors.New("invalid staged media")
			}
			return PublishResult{}, err
		}
		if err = s.Store.SaveStaging(ctx, draft.ID, staged); err != nil {
			return PublishResult{}, err
		}
		draft.StagingID, draft.StagingURL, draft.StagingExpiresAt = staged.ID, staged.URL, &staged.ExpiresAt
	}
	if draft.ContainerID == "" {
		containerID, createErr := s.Meta.CreateContainer(ctx, draft.AccountID, draft.StagingURL, draft.Caption, "instagram-container:"+draft.ID)
		if createErr != nil || containerID == "" {
			if createErr != nil {
				err = createErr
			} else {
				err = ErrContainer
			}
			return PublishResult{}, err
		}
		if err = s.Store.SaveContainer(ctx, draft.ID, containerID); err != nil {
			return PublishResult{}, err
		}
		draft.ContainerID = containerID
	}
	finished := false
	for poll := 0; poll < s.MaxPolls; poll++ {
		status, statusErr := s.Meta.ContainerStatus(ctx, draft.ContainerID)
		if statusErr != nil {
			err = statusErr
			return PublishResult{}, err
		}
		switch status {
		case ContainerFinished:
			finished = true
		case ContainerError, ContainerExpired:
			err = ErrContainer
			return PublishResult{}, err
		case ContainerInProgress:
			if sleepErr := s.Sleeper.Sleep(ctx, s.PollInterval); sleepErr != nil {
				err = sleepErr
				return PublishResult{}, err
			}
		default:
			err = ErrContainer
			return PublishResult{}, err
		}
		if finished {
			break
		}
	}
	if !finished {
		err = errors.New("Instagram container polling exhausted")
		return PublishResult{}, err
	}
	current, getErr := s.Store.Get(ctx, draft.ID)
	if getErr != nil {
		err = getErr
		return PublishResult{}, err
	}
	if current.AuthorizedUntil == nil || !s.Clock.Now().UTC().Before(*current.AuthorizedUntil) {
		err = ErrApprovalBinding
		return PublishResult{}, err
	}
	mediaID, publishErr := s.Meta.PublishContainer(ctx, draft.AccountID, draft.ContainerID, "instagram-publish:"+draft.ID)
	if publishErr != nil || mediaID == "" {
		if publishErr != nil {
			err = publishErr
		} else {
			err = ErrContainer
		}
		return PublishResult{}, err
	}
	if err = s.Store.Complete(ctx, draft.ID, mediaID); err != nil {
		return PublishResult{}, err
	}
	failed = false
	if cleanupErr := s.Stager.Cleanup(context.WithoutCancel(ctx), draft.StagingID); cleanupErr == nil {
		_ = s.Store.ClearStaging(context.WithoutCancel(ctx), draft.ID)
	} else {
		_ = s.record(context.WithoutCancel(ctx), "instagram_staging_cleanup", draft.AccountID, "deferred")
	}
	if err = s.record(ctx, "instagram_publish", draft.AccountID, "published_mock_or_external"); err != nil {
		return PublishResult{}, err
	}
	return PublishResult{DraftID: draft.ID, MediaID: mediaID, State: "published"}, nil
}

func (s *Service) Recover(ctx context.Context) (int64, error) {
	return s.Store.Recover(ctx)
}

func (s *Service) CleanupExpired(ctx context.Context) error {
	drafts, err := s.Store.ExpiredStaging(ctx)
	if err != nil {
		return err
	}
	for _, draft := range drafts {
		if err = s.Stager.Cleanup(ctx, draft.StagingID); err != nil {
			return err
		}
		if err = s.Store.ClearStaging(ctx, draft.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) validateDependencies() error {
	if s == nil || s.Store == nil || s.Artifacts == nil || s.Meta == nil || s.Stager == nil || s.Clock == nil || s.Sleeper == nil || s.OwnerID == 0 || s.PollInterval <= 0 || s.PollInterval > time.Minute || s.MaxPolls < 1 || s.MaxPolls > 60 {
		return errors.New("Instagram service dependencies or limits unavailable")
	}
	return nil
}

func bindingFor(draft Draft) PublishBinding {
	return PublishBinding{DraftID: draft.ID, AccountID: draft.AccountID, AssetHash: draft.AssetHash, CaptionHash: draft.CaptionHash, PublishAt: formatTime(draft.PublishAt)}
}

func canonicalBinding(binding PublishBinding) (json.RawMessage, error) {
	return json.Marshal(binding)
}

func validStagedMedia(media StagedMedia, now time.Time) bool {
	parsed, err := url.Parse(media.URL)
	return err == nil && media.ID != "" && parsed.Scheme == "https" && parsed.Hostname() != "" && parsed.User == nil && parsed.Fragment == "" && media.ExpiresAt.After(now.UTC().Add(time.Minute))
}

func (s *Service) record(ctx context.Context, kind, resource, decision string) error {
	if s.Audit == nil {
		return nil
	}
	_, err := s.Audit.Append(ctx, audit.Entry{Kind: kind, UserID: s.OwnerID, Resource: resource, Decision: decision})
	return err
}

func RegisterSkillHandler(registry *skills.Registry) error {
	if registry == nil {
		return errors.New("skill registry unavailable")
	}
	return registry.RegisterBuiltin("social-publishing", "1.0.0", func(ctx context.Context, skill *skills.SkillContext, input json.RawMessage) (any, error) {
		var request struct {
			Operation   string `json:"operation"`
			DraftID     string `json:"draft_id"`
			AccountID   string `json:"account_id"`
			ArtifactID  string `json:"artifact_id"`
			AssetHash   string `json:"asset_hash"`
			Caption     string `json:"caption"`
			CaptionHash string `json:"caption_hash"`
			PublishAt   string `json:"publish_at"`
			CreationKey string `json:"creation_key"`
			MaxAttempts int    `json:"max_attempts"`
		}
		if err := json.Unmarshal(input, &request); err != nil {
			return nil, ErrInvalidDraft
		}
		switch request.Operation {
		case "prepare":
			publishAt, err := time.Parse(time.RFC3339Nano, request.PublishAt)
			if err != nil {
				return nil, ErrInvalidDraft
			}
			arguments, _ := json.Marshal(PrepareRequest{AccountID: request.AccountID, ArtifactID: request.ArtifactID, Caption: request.Caption, PublishAt: publishAt, CreationKey: request.CreationKey, MaxAttempts: request.MaxAttempts})
			return skill.CallTool(ctx, "instagram.prepare", policy.Write, arguments)
		case "preview":
			arguments, _ := json.Marshal(struct {
				DraftID string `json:"draft_id"`
			}{request.DraftID})
			return skill.CallTool(ctx, "instagram.preview", policy.Read, arguments)
		case "publish":
			arguments, _ := canonicalBinding(PublishBinding{DraftID: request.DraftID, AccountID: request.AccountID, AssetHash: request.AssetHash, CaptionHash: request.CaptionHash, PublishAt: request.PublishAt})
			return skill.CallTool(ctx, "instagram.publish", policy.Publish, arguments)
		default:
			return nil, ErrInvalidDraft
		}
	})
}

func RegisteredTools(service *Service) []tools.Registered {
	prepareSchema := json.RawMessage(`{"type":"object","properties":{"account_id":{"type":"string"},"artifact_id":{"type":"string"},"caption":{"type":"string"},"publish_at":{"type":"string"},"creation_key":{"type":"string"},"max_attempts":{"type":"integer"}},"required":["account_id","artifact_id","caption","publish_at","creation_key"],"additionalProperties":false}`)
	previewSchema := json.RawMessage(`{"type":"object","properties":{"draft_id":{"type":"string"}},"required":["draft_id"],"additionalProperties":false}`)
	publishSchema := json.RawMessage(`{"type":"object","properties":{"draft_id":{"type":"string"},"account_id":{"type":"string"},"asset_hash":{"type":"string"},"caption_hash":{"type":"string"},"publish_at":{"type":"string"}},"required":["draft_id","account_id","asset_hash","caption_hash","publish_at"],"additionalProperties":false}`)
	decode := func(raw json.RawMessage, target any) error {
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(target); err != nil {
			return err
		}
		var trailing any
		if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
			return ErrInvalidDraft
		}
		return nil
	}
	return []tools.Registered{
		{Spec: policy.Tool{Name: "instagram.prepare", Version: "1.0.0", InputSchema: prepareSchema, Risk: policy.Medium, Modes: []policy.Mode{policy.Write}, Timeout: 10 * time.Second, OutputLimit: 16 * 1024, Idempotency: policy.Idempotent}, Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var request PrepareRequest
			if err := decode(raw, &request); err != nil {
				return nil, ErrInvalidDraft
			}
			return service.Prepare(ctx, request)
		}},
		{Spec: policy.Tool{Name: "instagram.preview", Version: "1.0.0", InputSchema: previewSchema, Risk: policy.Safe, Modes: []policy.Mode{policy.Read}, Timeout: 5 * time.Second, OutputLimit: 16 * 1024, Idempotency: policy.Idempotent}, Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var request struct {
				DraftID string `json:"draft_id"`
			}
			if err := decode(raw, &request); err != nil {
				return nil, ErrInvalidDraft
			}
			return service.Preview(ctx, request.DraftID)
		}},
		{Spec: policy.Tool{Name: "instagram.publish", Version: "1.0.0", InputSchema: publishSchema, Risk: policy.Critical, Modes: []policy.Mode{policy.Publish}, Approval: true, Timeout: 5 * time.Minute, OutputLimit: 16 * 1024, NetworkScope: []string{"graph.facebook.com"}, Idempotency: policy.AtLeastOnce}, Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			var binding PublishBinding
			if err := decode(raw, &binding); err != nil {
				return nil, ErrInvalidDraft
			}
			metadata, ok := tools.Metadata(ctx)
			if !ok || metadata.Approval == nil {
				return nil, ErrApprovalBinding
			}
			return service.PublishApproved(ctx, binding, *metadata.Approval)
		}},
	}
}
