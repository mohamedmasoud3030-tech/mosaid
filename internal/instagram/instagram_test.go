package instagram

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/approval"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/audit"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/images"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/policy"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/storage"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/tools"
)

type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(duration time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(duration)
	c.mu.Unlock()
}

type fakeSleeper struct{ clock *fakeClock }

func (s fakeSleeper) Sleep(ctx context.Context, duration time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.clock.Advance(duration)
	return nil
}

type fakeStager struct {
	mu           sync.Mutex
	clock        *fakeClock
	stageCalls   int
	cleanupCalls int
	cleanupIDs   []string
}

func (s *fakeStager) Stage(_ context.Context, artifactID, _ string, _ []byte, _ time.Time) (StagedMedia, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stageCalls++
	return StagedMedia{ID: "staged-" + artifactID[:8], URL: "https://media.example.invalid/staged/" + artifactID, ExpiresAt: s.clock.Now().Add(2 * time.Hour)}, nil
}

func (s *fakeStager) Cleanup(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanupCalls++
	s.cleanupIDs = append(s.cleanupIDs, id)
	return nil
}

type fakeMeta struct {
	mu              sync.Mutex
	statuses        []ContainerStatus
	createCalls     int
	statusCalls     int
	publishCalls    int
	publishFailures int
	createKeys      []string
	publishKeys     []string
}

func (m *fakeMeta) CreateContainer(_ context.Context, _, _, _ string, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.createCalls++
	m.createKeys = append(m.createKeys, key)
	return "container-1", nil
}

func (m *fakeMeta) ContainerStatus(context.Context, string) (ContainerStatus, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusCalls++
	if len(m.statuses) == 0 {
		return ContainerFinished, nil
	}
	index := m.statusCalls - 1
	if index >= len(m.statuses) {
		index = len(m.statuses) - 1
	}
	return m.statuses[index], nil
}

func (m *fakeMeta) PublishContainer(_ context.Context, _, _ string, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.publishCalls++
	m.publishKeys = append(m.publishKeys, key)
	if m.publishFailures > 0 {
		m.publishFailures--
		return "", errors.New("temporary Meta failure")
	}
	return "media-1", nil
}

func pngArtifact(t *testing.T) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, 256, 256))
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			canvas.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 90, A: 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, canvas); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

type serviceFixture struct {
	service    *Service
	db         *storage.DB
	clock      *fakeClock
	meta       *fakeMeta
	stager     *fakeStager
	artifactID string
}

func newFixture(t *testing.T) serviceFixture {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "instagram.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	artifactStore, err := images.NewStore(filepath.Join(t.TempDir(), "artifacts"))
	if err != nil {
		t.Fatal(err)
	}
	artifactID, _, err := artifactStore.Put(pngArtifact(t), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	clock := &fakeClock{now: time.Now().UTC().Truncate(time.Second)}
	meta := &fakeMeta{statuses: []ContainerStatus{ContainerInProgress, ContainerFinished}}
	stager := &fakeStager{clock: clock}
	logger := &audit.Logger{DB: db.SQL()}
	approvals := &approval.Manager{DB: db.SQL(), Audit: *logger}
	service := &Service{
		Store: NewStore(db.SQL(), clock), Artifacts: artifactStore, Meta: meta, Stager: stager, Approvals: approvals,
		Audit: logger, Clock: clock, Sleeper: fakeSleeper{clock: clock}, OwnerID: 42, PollInterval: time.Second, MaxPolls: 5,
	}
	return serviceFixture{service: service, db: db, clock: clock, meta: meta, stager: stager, artifactID: artifactID}
}

func prepareDraft(t *testing.T, fixture serviceFixture, key string) Draft {
	t.Helper()
	draft, err := fixture.service.Prepare(context.Background(), PrepareRequest{
		AccountID: "17890000123456789", ArtifactID: fixture.artifactID, Caption: "A mock-only preview caption",
		PublishAt: fixture.clock.Now(), CreationKey: key, MaxAttempts: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	return draft
}

func registerInstagramTools(t *testing.T, fixture serviceFixture) *tools.Registry {
	t.Helper()
	registry := tools.NewRegistry(fixture.service.Approvals)
	for _, tool := range RegisteredTools(fixture.service) {
		if err := registry.Register(tool); err != nil {
			t.Fatal(err)
		}
	}
	return registry
}

func TestPreparePreviewAndPublishRequireBoundApproval(t *testing.T) {
	fixture := newFixture(t)
	draft := prepareDraft(t, fixture, "prepare-1")
	preview, err := fixture.service.Preview(context.Background(), draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Caption != draft.Caption || preview.Binding.AssetHash != fixture.artifactID || !strings.Contains(preview.Warning, "does not authorize") {
		t.Fatalf("preview=%+v", preview)
	}
	registry := registerInstagramTools(t, fixture)
	arguments, _ := canonicalBinding(preview.Binding)
	request := tools.Request{Name: "instagram.publish", Arguments: arguments, Mode: policy.Publish, UserID: 42, Resource: draft.AccountID}
	first, err := registry.Execute(context.Background(), request)
	if err != nil || first.Approval == nil || fixture.meta.publishCalls != 0 {
		t.Fatalf("first=%+v err=%v calls=%d", first, err, fixture.meta.publishCalls)
	}
	if err = fixture.service.Approvals.ResolveToken(context.Background(), first.Approval.Token, 42, "approved"); err != nil {
		t.Fatal(err)
	}
	request.ApprovalToken = first.Approval.Token
	second, err := registry.Execute(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	result, ok := second.Value.(PublishResult)
	if !ok || result.State != "published" || result.MediaID != "media-1" {
		t.Fatalf("result=%+v", second.Value)
	}
	if fixture.meta.createCalls != 1 || fixture.meta.publishCalls != 1 || fixture.stager.stageCalls != 1 || fixture.stager.cleanupCalls != 1 {
		t.Fatalf("meta=%+v stager=%+v", fixture.meta, fixture.stager)
	}
	if fixture.meta.createKeys[0] != "instagram-container:"+draft.ID || fixture.meta.publishKeys[0] != "instagram-publish:"+draft.ID {
		t.Fatalf("keys create=%v publish=%v", fixture.meta.createKeys, fixture.meta.publishKeys)
	}
	stored, err := fixture.service.Store.Get(context.Background(), draft.ID)
	if err != nil || stored.State != "published" || stored.MediaID != "media-1" || stored.StagingURL != "" {
		t.Fatalf("stored=%+v err=%v", stored, err)
	}
	if err = fixture.service.Audit.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestPublishApprovalBindsEverySensitiveField(t *testing.T) {
	fields := []string{"account", "asset", "caption", "time"}
	for _, field := range fields {
		t.Run(field, func(t *testing.T) {
			fixture := newFixture(t)
			draft := prepareDraft(t, fixture, "binding-"+field)
			binding := bindingFor(draft)
			arguments, _ := canonicalBinding(binding)
			registry := registerInstagramTools(t, fixture)
			request := tools.Request{Name: "instagram.publish", Arguments: arguments, Mode: policy.Publish, UserID: 42, Resource: draft.AccountID}
			first, err := registry.Execute(context.Background(), request)
			if err != nil || first.Approval == nil {
				t.Fatal(err)
			}
			if err = fixture.service.Approvals.ResolveToken(context.Background(), first.Approval.Token, 42, "approved"); err != nil {
				t.Fatal(err)
			}
			switch field {
			case "account":
				binding.AccountID = "999"
			case "asset":
				binding.AssetHash = strings.Repeat("0", 64)
			case "caption":
				binding.CaptionHash = strings.Repeat("1", 64)
			case "time":
				binding.PublishAt = time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
			}
			request.Arguments, _ = canonicalBinding(binding)
			request.ApprovalToken = first.Approval.Token
			if _, err = registry.Execute(context.Background(), request); err == nil {
				t.Fatal("tampered binding accepted")
			}
			if fixture.meta.publishCalls != 0 {
				t.Fatal("Meta called for tampered approval")
			}
		})
	}
}

func TestRetryRecoveryReusesContainerAndLogicalIdempotency(t *testing.T) {
	fixture := newFixture(t)
	fixture.meta.statuses = []ContainerStatus{ContainerFinished}
	fixture.meta.publishFailures = 1
	draft := prepareDraft(t, fixture, "retry-1")
	approvalRequest, _, err := fixture.service.RequestApproval(context.Background(), draft.ID, 42, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err = fixture.service.Approvals.ResolveToken(context.Background(), approvalRequest.Token, 42, "approved"); err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.PublishWithToken(context.Background(), draft.ID, 42, approvalRequest.Token); err == nil {
		t.Fatal("expected first publish failure")
	}
	failed, _ := fixture.service.Store.Get(context.Background(), draft.ID)
	if failed.State != "failed" || failed.ContainerID == "" || failed.AuthorizedUntil == nil {
		t.Fatalf("failed=%+v", failed)
	}
	fixture.clock.Advance(2 * time.Second)
	result, err := fixture.service.Resume(context.Background(), draft.ID)
	if err != nil || result.State != "published" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if fixture.meta.createCalls != 1 || fixture.meta.publishCalls != 2 || fixture.meta.publishKeys[0] != fixture.meta.publishKeys[1] {
		t.Fatalf("create=%d publish=%d keys=%v", fixture.meta.createCalls, fixture.meta.publishCalls, fixture.meta.publishKeys)
	}
	again, err := fixture.service.Resume(context.Background(), draft.ID)
	if err != nil || again.MediaID != result.MediaID || fixture.meta.publishCalls != 2 {
		t.Fatalf("again=%+v err=%v calls=%d", again, err, fixture.meta.publishCalls)
	}
}

func TestRestartRecoveryPreservesContainer(t *testing.T) {
	fixture := newFixture(t)
	draft := prepareDraft(t, fixture, "restart-1")
	if err := fixture.service.Store.MarkAuthorized(context.Background(), draft.ID, fixture.clock.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	claimed, err := fixture.service.Store.Claim(context.Background(), draft.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err = fixture.service.Store.SaveContainer(context.Background(), claimed.ID, "container-existing"); err != nil {
		t.Fatal(err)
	}
	recovered, err := fixture.service.Recover(context.Background())
	if err != nil || recovered != 1 {
		t.Fatalf("recovered=%d err=%v", recovered, err)
	}
	fixture.clock.Advance(2 * time.Second)
	result, err := fixture.service.Resume(context.Background(), draft.ID)
	if err != nil || result.State != "published" || fixture.meta.createCalls != 0 {
		t.Fatalf("result=%+v err=%v create=%d", result, err, fixture.meta.createCalls)
	}
}

func TestPrepareIdempotencyAndConflict(t *testing.T) {
	fixture := newFixture(t)
	first := prepareDraft(t, fixture, "same-key")
	second := prepareDraft(t, fixture, "same-key")
	if first.ID != second.ID {
		t.Fatalf("ids %s %s", first.ID, second.ID)
	}
	_, err := fixture.service.Prepare(context.Background(), PrepareRequest{AccountID: first.AccountID, ArtifactID: fixture.artifactID, Caption: "changed caption", PublishAt: fixture.clock.Now(), CreationKey: "same-key", MaxAttempts: 3})
	if !errors.Is(err, ErrDraftConflict) {
		t.Fatalf("err=%v", err)
	}
}

func TestInvalidDraftAndNotDueAreRejected(t *testing.T) {
	fixture := newFixture(t)
	_, err := fixture.service.Prepare(context.Background(), PrepareRequest{AccountID: "not-an-id", ArtifactID: fixture.artifactID, Caption: "x", PublishAt: fixture.clock.Now(), CreationKey: "bad"})
	if !errors.Is(err, ErrInvalidDraft) {
		t.Fatalf("err=%v", err)
	}
	draft, err := fixture.service.Prepare(context.Background(), PrepareRequest{AccountID: "123456", ArtifactID: fixture.artifactID, Caption: "future", PublishAt: fixture.clock.Now().Add(time.Hour), CreationKey: "future", MaxAttempts: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err = fixture.service.Store.MarkAuthorized(context.Background(), draft.ID, fixture.clock.Now().Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err = fixture.service.Resume(context.Background(), draft.ID); !errors.Is(err, ErrNotDue) {
		t.Fatalf("err=%v", err)
	}
}

type graphToken struct{}

func (graphToken) BearerToken(context.Context) (string, error) { return "mock-meta-credential", nil }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestExpiredMediaStagingCleanupPolicy(t *testing.T) {
	fixture := newFixture(t)
	draft := prepareDraft(t, fixture, "cleanup-1")
	if err := fixture.service.Store.MarkAuthorized(context.Background(), draft.ID, fixture.clock.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.Store.Claim(context.Background(), draft.ID); err != nil {
		t.Fatal(err)
	}
	media := StagedMedia{ID: "expired-staging", URL: "https://media.example.invalid/expired", ExpiresAt: fixture.clock.Now().Add(-time.Minute)}
	if err := fixture.service.Store.SaveStaging(context.Background(), draft.ID, media); err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.Store.Fail(context.Background(), draft.ID, errors.New("defer cleanup")); err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.CleanupExpired(context.Background()); err != nil {
		t.Fatal(err)
	}
	stored, err := fixture.service.Store.Get(context.Background(), draft.ID)
	if err != nil || stored.StagingID != "" || fixture.stager.cleanupCalls != 1 {
		t.Fatalf("stored=%+v cleanup=%d err=%v", stored, fixture.stager.cleanupCalls, err)
	}
}

func TestOfficialMetaGraphAPIContract(t *testing.T) {
	var calls []string
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Host != "graph.facebook.com" || request.Header.Get("Authorization") != "Bearer mock-meta-credential" || len(request.Cookies()) != 0 {
			t.Fatalf("unsafe request host=%s auth=%q cookies=%v", request.URL.Host, request.Header.Get("Authorization"), request.Cookies())
		}
		calls = append(calls, request.Method+" "+request.URL.Path)
		var payload string
		switch {
		case strings.HasSuffix(request.URL.Path, "/media"):
			body, _ := io.ReadAll(request.Body)
			values, _ := url.ParseQuery(string(body))
			if values.Get("image_url") == "" || values.Get("caption") != "caption" || request.Header.Get("Idempotency-Key") != "container-key" {
				t.Fatalf("media values=%v key=%q", values, request.Header.Get("Idempotency-Key"))
			}
			payload = `{"id":"container-graph"}`
		case strings.HasSuffix(request.URL.Path, "/media_publish"):
			body, _ := io.ReadAll(request.Body)
			values, _ := url.ParseQuery(string(body))
			if values.Get("creation_id") != "container-graph" || request.Header.Get("Idempotency-Key") != "publish-key" {
				t.Fatalf("publish values=%v", values)
			}
			payload = `{"id":"media-graph"}`
		default:
			if request.URL.Query().Get("fields") != "status_code" {
				t.Fatalf("status query=%v", request.URL.Query())
			}
			payload = `{"status_code":"FINISHED"}`
		}
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(payload))}, nil
	})}
	graph, err := NewGraphClient(MetaConfig{APIVersion: "v99.0", Tokens: graphToken{}, Client: client})
	if err != nil {
		t.Fatal(err)
	}
	container, err := graph.CreateContainer(context.Background(), "123456", "https://media.example.invalid/image.png", "caption", "container-key")
	if err != nil || container != "container-graph" {
		t.Fatalf("container=%s err=%v", container, err)
	}
	status, err := graph.ContainerStatus(context.Background(), container)
	if err != nil || status != ContainerFinished {
		t.Fatalf("status=%s err=%v", status, err)
	}
	media, err := graph.PublishContainer(context.Background(), "123456", container, "publish-key")
	if err != nil || media != "media-graph" || len(calls) != 3 {
		t.Fatalf("media=%s calls=%v err=%v", media, calls, err)
	}
}

func TestDraftJSONNeverExposesStagingURL(t *testing.T) {
	draft := Draft{StagingURL: "https://media.example.invalid/private-signed-url"}
	encoded, err := json.Marshal(draft)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encoded, []byte("private-signed-url")) {
		t.Fatalf("draft leaked staging URL: %s", encoded)
	}
}
