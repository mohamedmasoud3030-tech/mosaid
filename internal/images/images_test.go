package images

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/approval"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/audit"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/policy"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/storage"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/tools"
)

type fixedClock struct{ value time.Time }

func (c fixedClock) Now() time.Time { return c.value }

func pngImage(t *testing.T, width, height int) []byte {
	t.Helper()
	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			canvas.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 80, A: 255})
		}
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, canvas); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

type mockProvider struct {
	mu     sync.Mutex
	result ProviderResult
	err    error
	calls  int
	last   ProviderRequest
}

func (p *mockProvider) Generate(_ context.Context, request ProviderRequest) (ProviderResult, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.calls++
	p.last = request
	return p.result, p.err
}

func testService(t *testing.T, provider Provider) (*Service, *storage.DB) {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "images.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	artifacts, err := NewStore(filepath.Join(t.TempDir(), "artifacts"))
	if err != nil {
		t.Fatal(err)
	}
	logger := &audit.Logger{DB: db.SQL()}
	return &Service{Provider: provider, Artifacts: artifacts, Audit: logger, Clock: fixedClock{value: time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)}, OwnerID: 42, NetworkScope: []string{"images.example"}}, db
}

func validRequest() GenerationRequest {
	return GenerationRequest{Prompt: "A small geometric landscape", Width: 256, Height: 256, AspectRatio: "1:1", MaxCostUSD: 1}
}

func validResult(t *testing.T) ProviderResult {
	return ProviderResult{Data: pngImage(t, 256, 256), MIME: "image/png", Provider: "mock", Model: "mock-v1", Cost: Cost{Currency: "USD", Amount: 0.04, Units: 1}}
}

func TestGenerationValidatesStoresHashesAuditsAndNeverPublishes(t *testing.T) {
	provider := &mockProvider{result: validResult(t)}
	service, db := testService(t, provider)
	artifact, err := service.Generate(context.Background(), validRequest())
	if err != nil {
		t.Fatal(err)
	}
	if artifact.ID == "" || artifact.ID != artifact.SHA256 || artifact.MIME != "image/png" || artifact.Width != 256 || artifact.Height != 256 || artifact.Cost.Amount != 0.04 || artifact.Publish {
		t.Fatalf("artifact=%+v", artifact)
	}
	reference, err := service.Artifacts.Read(artifact.ID)
	if err != nil || reference.SHA256 != artifact.SHA256 || !bytes.Equal(reference.Data, provider.result.Data) {
		t.Fatalf("reference=%+v err=%v", reference, err)
	}
	var audits int
	if err = db.SQL().QueryRow(`SELECT count(*) FROM audit_entries WHERE kind IN('image_generation','image_artifact')`).Scan(&audits); err != nil || audits != 2 {
		t.Fatalf("audits=%d err=%v", audits, err)
	}
	if err = service.Audit.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestReferenceAssetsAreIntegrityCheckedAndPassedToProvider(t *testing.T) {
	provider := &mockProvider{result: validResult(t)}
	service, _ := testService(t, provider)
	id, _, err := service.Artifacts.Put(pngImage(t, 256, 256), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	request := validRequest()
	request.ReferenceIDs = []string{id}
	if _, err = service.Generate(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if len(provider.last.References) != 1 || provider.last.References[0].ID != id || provider.last.References[0].MIME != "image/png" {
		t.Fatalf("references=%+v", provider.last.References)
	}
}

func TestInvalidProviderImageAndCostAreRejected(t *testing.T) {
	tests := []struct {
		name   string
		result func(*testing.T) ProviderResult
		want   error
	}{
		{"mime mismatch", func(t *testing.T) ProviderResult { r := validResult(t); r.MIME = "image/jpeg"; return r }, ErrInvalidImage},
		{"dimensions", func(t *testing.T) ProviderResult { r := validResult(t); r.Data = pngImage(t, 512, 512); return r }, ErrInvalidImage},
		{"malformed", func(*testing.T) ProviderResult {
			return ProviderResult{Data: []byte("not-image"), MIME: "image/png", Provider: "mock", Model: "v1", Cost: Cost{Currency: "USD", Units: 1}}
		}, ErrInvalidImage},
		{"cost", func(t *testing.T) ProviderResult { r := validResult(t); r.Cost.Amount = 2; return r }, ErrCostBudget},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &mockProvider{result: test.result(t)}
			service, _ := testService(t, provider)
			_, err := service.Generate(context.Background(), validRequest())
			if !errors.Is(err, test.want) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestRequestValidation(t *testing.T) {
	provider := &mockProvider{result: validResult(t)}
	service, _ := testService(t, provider)
	tests := []GenerationRequest{
		{Prompt: "", Width: 256, Height: 256, AspectRatio: "1:1", MaxCostUSD: 1},
		{Prompt: "x", Width: 257, Height: 256, AspectRatio: "1:1", MaxCostUSD: 1},
		{Prompt: "x", Width: 256, Height: 256, AspectRatio: "16:9", MaxCostUSD: 1},
		{Prompt: "x", Width: 256, Height: 256, AspectRatio: "1:1", MaxCostUSD: 0},
	}
	for _, request := range tests {
		if _, err := service.Generate(context.Background(), request); !errors.Is(err, ErrInvalidRequest) {
			t.Fatalf("request=%+v err=%v", request, err)
		}
	}
	if provider.calls != 0 {
		t.Fatalf("provider calls=%d", provider.calls)
	}
}

func TestArtifactStoreRejectsSymlinksAndMalformedIDs(t *testing.T) {
	store, err := NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data := pngImage(t, 256, 256)
	id := strings.Repeat("a", 64)
	outside := filepath.Join(t.TempDir(), "outside.png")
	if err = os.WriteFile(outside, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(outside, filepath.Join(store.root, id+".png")); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Read(id); !errors.Is(err, ErrArtifact) {
		t.Fatalf("symlink err=%v", err)
	}
	if _, err = store.Read("../escape"); !errors.Is(err, ErrArtifact) {
		t.Fatalf("id err=%v", err)
	}
}

func TestImageToolRequiresCorePolicyApproval(t *testing.T) {
	provider := &mockProvider{result: validResult(t)}
	service, db := testService(t, provider)
	manager := &approval.Manager{DB: db.SQL(), Audit: audit.Logger{DB: db.SQL()}}
	registry := tools.NewRegistry(manager)
	if err := registry.Register(RegisteredTool(service)); err != nil {
		t.Fatal(err)
	}
	arguments, _ := json.Marshal(validRequest())
	request := tools.Request{Name: "images.generate", Arguments: arguments, Mode: policy.Write, UserID: 42, Resource: "image-task"}
	first, err := registry.Execute(context.Background(), request)
	if err != nil || first.Approval == nil || provider.calls != 0 {
		t.Fatalf("first=%+v calls=%d err=%v", first, provider.calls, err)
	}
	if err = manager.ResolveToken(context.Background(), first.Approval.Token, 42, "approved"); err != nil {
		t.Fatal(err)
	}
	request.ApprovalToken = first.Approval.Token
	second, err := registry.Execute(context.Background(), request)
	if err != nil || second.Value == nil || provider.calls != 1 {
		t.Fatalf("second=%+v calls=%d err=%v", second, provider.calls, err)
	}
}

type tokenSource struct{ value string }

func (t tokenSource) BearerToken(context.Context) (string, error) { return t.value, nil }

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestOpenAICompatibleProviderContract(t *testing.T) {
	imageData := pngImage(t, 256, 256)
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Method != http.MethodPost || request.URL.String() != "https://images.example/v1/images/generations" || request.Header.Get("Authorization") != "Bearer test-credential" {
			t.Fatalf("request=%s %s auth=%q", request.Method, request.URL, request.Header.Get("Authorization"))
		}
		body, _ := io.ReadAll(request.Body)
		if !bytes.Contains(body, []byte(`"response_format":"b64_json"`)) || !bytes.Contains(body, []byte(`"size":"256x256"`)) {
			t.Fatalf("body=%s", body)
		}
		response := map[string]any{
			"created": 1,
			"data":    []map[string]any{{"b64_json": base64.StdEncoding.EncodeToString(imageData), "mime_type": "image/png", "revised_prompt": "safe"}},
			"usage":   map[string]any{"cost_usd": 0.03, "images": 1},
		}
		encoded, _ := json.Marshal(response)
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(encoded))}, nil
	})}
	provider, err := NewOpenAIProvider(OpenAIConfig{Endpoint: "https://images.example/v1", Model: "image-model-v1", Tokens: tokenSource{value: "test-credential"}, Client: client})
	if err != nil {
		t.Fatal(err)
	}
	result, err := provider.Generate(context.Background(), ProviderRequest{Prompt: "test", Width: 256, Height: 256, AspectRatio: "1:1", MaxCostUSD: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.MIME != "image/png" || result.Model != "image-model-v1" || result.Cost.Amount != 0.03 || !bytes.Equal(result.Data, imageData) {
		t.Fatalf("result=%+v", result)
	}
}
