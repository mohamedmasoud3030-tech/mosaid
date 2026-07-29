package images

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/audit"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/policy"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/tools"
)

type Service struct {
	Provider     Provider
	Artifacts    *Store
	Audit        *audit.Logger
	Clock        Clock
	OwnerID      int64
	NetworkScope []string
}

func (s *Service) Generate(ctx context.Context, request GenerationRequest) (Artifact, error) {
	if s == nil || s.Provider == nil || s.Artifacts == nil || s.Clock == nil || s.OwnerID == 0 {
		return Artifact{}, errors.New("image service dependencies unavailable")
	}
	if err := validateRequest(request); err != nil {
		return Artifact{}, err
	}
	references := make([]Reference, 0, len(request.ReferenceIDs))
	totalReferenceBytes := 0
	for _, id := range request.ReferenceIDs {
		reference, err := s.Artifacts.Read(id)
		if err != nil {
			return Artifact{}, fmt.Errorf("reference %s: %w", id, err)
		}
		totalReferenceBytes += len(reference.Data)
		if totalReferenceBytes > 20*1024*1024 {
			return Artifact{}, fmt.Errorf("%w: references too large", ErrInvalidRequest)
		}
		references = append(references, reference)
	}
	if err := s.record(ctx, "image_generation", "requested"); err != nil {
		return Artifact{}, err
	}
	result, err := s.Provider.Generate(ctx, ProviderRequest{
		Prompt: request.Prompt, Width: request.Width, Height: request.Height, AspectRatio: request.AspectRatio,
		References: references, MaxCostUSD: request.MaxCostUSD,
	})
	if err != nil {
		_ = s.record(context.WithoutCancel(ctx), "image_generation", "provider_error")
		return Artifact{}, err
	}
	if err = validateProviderResult(result, request); err != nil {
		_ = s.record(context.WithoutCancel(ctx), "image_generation", "rejected_output")
		return Artifact{}, err
	}
	if result.Cost.Amount > request.MaxCostUSD {
		_ = s.record(context.WithoutCancel(ctx), "image_generation", "cost_denied")
		return Artifact{}, ErrCostBudget
	}
	id, path, err := s.Artifacts.Put(result.Data, result.MIME)
	if err != nil {
		return Artifact{}, err
	}
	artifact := Artifact{
		ID: id, Path: path, MIME: result.MIME, Bytes: len(result.Data), Width: request.Width, Height: request.Height,
		SHA256: id, Provider: result.Provider, Model: result.Model, Cost: result.Cost, CreatedAt: s.Clock.Now().UTC(), Publish: false,
	}
	if err = s.record(ctx, "image_artifact", id); err != nil {
		return Artifact{}, err
	}
	return artifact, nil
}

func validateRequest(request GenerationRequest) error {
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" || prompt != request.Prompt || len(prompt) > 4000 || strings.ContainsRune(prompt, 0) {
		return fmt.Errorf("%w: prompt", ErrInvalidRequest)
	}
	if request.Width < 256 || request.Width > 2048 || request.Height < 256 || request.Height > 2048 || request.Width%64 != 0 || request.Height%64 != 0 {
		return fmt.Errorf("%w: dimensions", ErrInvalidRequest)
	}
	if !aspectMatches(request.AspectRatio, request.Width, request.Height) {
		return fmt.Errorf("%w: aspect ratio", ErrInvalidRequest)
	}
	if len(request.ReferenceIDs) > 4 {
		return fmt.Errorf("%w: too many references", ErrInvalidRequest)
	}
	seen := map[string]struct{}{}
	for _, id := range request.ReferenceIDs {
		if len(id) != 64 {
			return fmt.Errorf("%w: reference id", ErrInvalidRequest)
		}
		if _, exists := seen[id]; exists {
			return fmt.Errorf("%w: duplicate reference", ErrInvalidRequest)
		}
		seen[id] = struct{}{}
	}
	if math.IsNaN(request.MaxCostUSD) || math.IsInf(request.MaxCostUSD, 0) || request.MaxCostUSD <= 0 || request.MaxCostUSD > 100 {
		return fmt.Errorf("%w: cost budget", ErrInvalidRequest)
	}
	return nil
}

func validateProviderResult(result ProviderResult, request GenerationRequest) error {
	if len(result.Data) == 0 || len(result.Data) > maxArtifactBytes || result.Provider == "" || result.Model == "" {
		return ErrInvalidImage
	}
	if result.MIME != "image/png" && result.MIME != "image/jpeg" {
		return ErrInvalidImage
	}
	if detected := http.DetectContentType(result.Data); detected != result.MIME {
		return ErrInvalidImage
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(result.Data))
	if err != nil || config.Width != request.Width || config.Height != request.Height || (format == "png" && result.MIME != "image/png") || (format == "jpeg" && result.MIME != "image/jpeg") {
		return ErrInvalidImage
	}
	if result.Cost.Currency != "USD" || result.Cost.Amount < 0 || math.IsNaN(result.Cost.Amount) || math.IsInf(result.Cost.Amount, 0) || result.Cost.Units < 1 {
		return ErrInvalidImage
	}
	return nil
}

func aspectMatches(aspect string, width, height int) bool {
	switch aspect {
	case "1:1":
		return width == height
	case "16:9":
		return width*9 == height*16
	case "9:16":
		return width*16 == height*9
	case "4:3":
		return width*3 == height*4
	case "3:4":
		return width*4 == height*3
	default:
		return false
	}
}

func (s *Service) record(ctx context.Context, kind, decision string) error {
	if s.Audit == nil {
		return nil
	}
	_, err := s.Audit.Append(ctx, audit.Entry{Kind: kind, UserID: s.OwnerID, Resource: "images", Decision: decision})
	return err
}

func RegisteredTool(service *Service) tools.Registered {
	schema := json.RawMessage(`{"type":"object","properties":{"prompt":{"type":"string"},"width":{"type":"integer"},"height":{"type":"integer"},"aspect_ratio":{"type":"string"},"reference_ids":{"type":"array"},"max_cost_usd":{"type":"number"}},"required":["prompt","width","height","aspect_ratio","max_cost_usd"],"additionalProperties":false}`)
	return tools.Registered{
		Spec: policy.Tool{
			Name: "images.generate", Version: "1.0.0", InputSchema: schema, Risk: policy.Medium, Modes: []policy.Mode{policy.Write}, Approval: true,
			Timeout: 3 * time.Minute, OutputLimit: 16 * 1024, NetworkScope: append([]string(nil), service.NetworkScope...), Idempotency: policy.NonRepeatable,
		},
		Run: func(ctx context.Context, raw json.RawMessage) (any, error) {
			decoder := json.NewDecoder(bytes.NewReader(raw))
			decoder.DisallowUnknownFields()
			var request GenerationRequest
			if err := decoder.Decode(&request); err != nil {
				return nil, ErrInvalidRequest
			}
			var trailing any
			if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
				return nil, ErrInvalidRequest
			}
			return service.Generate(ctx, request)
		},
	}
}
