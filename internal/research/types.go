package research

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"
)

const TrustLabel = "UNTRUSTED_EXTERNAL_CONTENT"

var (
	ErrURLDenied       = errors.New("external URL denied")
	ErrAddressDenied   = errors.New("external address denied")
	ErrRedirectDenied  = errors.New("external redirect denied")
	ErrContentTooLarge = errors.New("external content exceeds size limit")
	ErrContentType     = errors.New("external content type denied")
)

type Clock interface {
	Now() time.Time
}

type RealClock struct{}

func (RealClock) Now() time.Time { return time.Now().UTC() }

type Provenance struct {
	URL         string    `json:"url"`
	RetrievedAt time.Time `json:"retrieved_at"`
	SHA256      string    `json:"sha256"`
	ContentType string    `json:"content_type"`
	Bytes       int       `json:"bytes"`
}

type ExternalContent struct {
	Trust              string     `json:"trust"`
	Text               string     `json:"text"`
	Provenance         Provenance `json:"provenance"`
	CanChangePolicy    bool       `json:"can_change_policy"`
	CanRequestSecrets  bool       `json:"can_request_secrets"`
	CanApproveActions  bool       `json:"can_approve_actions"`
	CanRunTools        bool       `json:"can_run_tools"`
	CanPersistLongTerm bool       `json:"can_persist_long_term"`
}

func NewExternalContent(text string, provenance Provenance) ExternalContent {
	return ExternalContent{Trust: TrustLabel, Text: text, Provenance: provenance}
}

func HashBytes(content []byte) string {
	hash := sha256.Sum256(content)
	return hex.EncodeToString(hash[:])
}

type SearchQuery struct {
	Text  string
	Limit int
}

type ProviderResult struct {
	Title   string
	URL     string
	Snippet string
}

type SearchProvider interface {
	Search(context.Context, SearchQuery) ([]ProviderResult, error)
}

type SearchResult struct {
	Title      string          `json:"title"`
	URL        string          `json:"url"`
	Snippet    ExternalContent `json:"snippet"`
	ProviderID string          `json:"provider_id"`
}
