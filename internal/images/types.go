package images

import (
	"context"
	"errors"
	"time"
)

var (
	ErrInvalidRequest = errors.New("invalid image generation request")
	ErrInvalidImage   = errors.New("invalid generated image")
	ErrCostBudget     = errors.New("image generation cost budget exceeded")
	ErrArtifact       = errors.New("image artifact error")
)

type Cost struct {
	Currency string  `json:"currency"`
	Amount   float64 `json:"amount"`
	Units    int     `json:"units"`
}

type GenerationRequest struct {
	Prompt       string   `json:"prompt"`
	Width        int      `json:"width"`
	Height       int      `json:"height"`
	AspectRatio  string   `json:"aspect_ratio"`
	ReferenceIDs []string `json:"reference_ids,omitempty"`
	MaxCostUSD   float64  `json:"max_cost_usd"`
}

type Reference struct {
	ID     string
	MIME   string
	SHA256 string
	Data   []byte
}

type ProviderRequest struct {
	Prompt      string
	Width       int
	Height      int
	AspectRatio string
	References  []Reference
	MaxCostUSD  float64
}

type ProviderResult struct {
	Data     []byte
	MIME     string
	Provider string
	Model    string
	Cost     Cost
}

type Provider interface {
	Generate(context.Context, ProviderRequest) (ProviderResult, error)
}

type Artifact struct {
	ID        string    `json:"id"`
	Path      string    `json:"path"`
	MIME      string    `json:"mime"`
	Bytes     int       `json:"bytes"`
	Width     int       `json:"width"`
	Height    int       `json:"height"`
	SHA256    string    `json:"sha256"`
	Provider  string    `json:"provider"`
	Model     string    `json:"model"`
	Cost      Cost      `json:"cost"`
	CreatedAt time.Time `json:"created_at"`
	Publish   bool      `json:"publish"`
}

type Clock interface {
	Now() time.Time
}

type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now().UTC() }
