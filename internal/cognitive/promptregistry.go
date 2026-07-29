package cognitive

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sync"
	"time"
)

var (
	ErrPromptNotFound    = errors.New("prompt not found")
	ErrPromptConflict    = errors.New("prompt version already exists")
	ErrPromptIntegrity   = errors.New("prompt integrity verification failed")
	ErrPromptForbidden   = errors.New("prompt overrides forbidden system content")
	ErrInvalidPromptMeta = errors.New("invalid prompt metadata")
)

var promptIDPattern = regexp.MustCompile(`^[a-z][a-z0-9/-]{1,255}$`)

// PromptMetadata contains metadata about a registered prompt.
type PromptMetadata struct {
	ID                   string   `json:"id"`                    // e.g., "core/planning/v2"
	Version              string   `json:"version"`               // e.g., "2.1.0"
	Purpose              string   `json:"purpose"`               // Human-readable description
	IntegritySHA256      string   `json:"integrity_sha256"`      // SHA-256 of content
	LastReviewed         string   `json:"last_reviewed"`         // Date string
	RequiredCapabilities []string `json:"required_capabilities"` // e.g., ["structured-output"]
	AllowedTools         []string `json:"allowed_tools"`         // Tools this prompt can use
	Risk                 string   `json:"risk"`                  // "low", "medium", "high"
}

// PromptEntry represents a registered prompt with its content and metadata.
type PromptEntry struct {
	Metadata PromptMetadata
	Content  string
}

// PromptRegistry manages versioned prompts with integrity checking.
type PromptRegistry struct {
	mu      sync.RWMutex
	prompts map[string]map[string]PromptEntry // id -> version -> entry
}

// NewPromptRegistry creates a new prompt registry.
func NewPromptRegistry() *PromptRegistry {
	return &PromptRegistry{
		prompts: make(map[string]map[string]PromptEntry),
	}
}

// ForbiddenPrefixes are prompt ID prefixes that cannot be overridden by external content.
var ForbiddenPrefixes = []string{
	"system/identity",
	"system/policy",
	"system/approval",
	"system/tool-permissions",
	"system/secret-rules",
}

// Register adds a prompt to the registry.
func (r *PromptRegistry) Register(entry PromptEntry) error {
	if entry.Metadata.ID == "" {
		return fmt.Errorf("%w: prompt ID required", ErrInvalidPromptMeta)
	}
	if !promptIDPattern.MatchString(entry.Metadata.ID) {
		return fmt.Errorf("%w: invalid prompt ID %q", ErrInvalidPromptMeta, entry.Metadata.ID)
	}
	if entry.Metadata.Version == "" {
		return fmt.Errorf("%w: prompt version required", ErrInvalidPromptMeta)
	}
	if entry.Metadata.Purpose == "" || len(entry.Metadata.Purpose) > 1024 {
		return fmt.Errorf("%w: prompt purpose required (max 1024 chars)", ErrInvalidPromptMeta)
	}
	if entry.Content == "" {
		return fmt.Errorf("%w: prompt content required", ErrInvalidPromptMeta)
	}

	// Check if this ID is forbidden
	for _, prefix := range ForbiddenPrefixes {
		if entry.Metadata.ID == prefix || len(entry.Metadata.ID) > len(prefix) && entry.Metadata.ID[:len(prefix)] == prefix && entry.Metadata.ID[len(prefix)] == '/' {
			// Only allow system prompts from core, not external
			if entry.Metadata.Risk == "external" {
				return fmt.Errorf("%w: %q is a system prompt and cannot be overridden", ErrPromptForbidden, entry.Metadata.ID)
			}
		}
	}

	// Compute integrity hash
	hash := sha256.Sum256([]byte(entry.Content))
	computedHash := "sha256:" + hex.EncodeToString(hash[:])
	if entry.Metadata.IntegritySHA256 == "" {
		entry.Metadata.IntegritySHA256 = computedHash
	} else if entry.Metadata.IntegritySHA256 != computedHash {
		return fmt.Errorf("%w: expected %s, got %s", ErrPromptIntegrity, computedHash, entry.Metadata.IntegritySHA256)
	}

	if entry.Metadata.LastReviewed == "" {
		entry.Metadata.LastReviewed = time.Now().UTC().Format("2006-01-02")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	versions := r.prompts[entry.Metadata.ID]
	if versions == nil {
		versions = make(map[string]PromptEntry)
		r.prompts[entry.Metadata.ID] = versions
	}

	if _, exists := versions[entry.Metadata.Version]; exists {
		return fmt.Errorf("%w: %s@%s", ErrPromptConflict, entry.Metadata.ID, entry.Metadata.Version)
	}

	versions[entry.Metadata.Version] = entry
	return nil
}

// Get retrieves a prompt by ID and optional version.
func (r *PromptRegistry) Get(id, version string) (PromptEntry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	versions, exists := r.prompts[id]
	if !exists {
		return PromptEntry{}, fmt.Errorf("%w: %q", ErrPromptNotFound, id)
	}

	if version != "" {
		entry, exists := versions[version]
		if !exists {
			return PromptEntry{}, fmt.Errorf("%w: %s@%s", ErrPromptNotFound, id, version)
		}
		return entry, nil
	}

	// Return latest version (highest version number)
	var latest PromptEntry
	var latestVersion string
	for v, entry := range versions {
		if latestVersion == "" || v > latestVersion {
			latest = entry
			latestVersion = v
		}
	}
	return latest, nil
}

// List returns all registered prompt metadata.
func (r *PromptRegistry) List() []PromptMetadata {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []PromptMetadata
	for _, versions := range r.prompts {
		for _, entry := range versions {
			result = append(result, entry.Metadata)
		}
	}
	return result
}

// VerifyIntegrity checks if a prompt's content matches its registered hash.
func (r *PromptRegistry) VerifyIntegrity(id, version string) error {
	entry, err := r.Get(id, version)
	if err != nil {
		return err
	}

	hash := sha256.Sum256([]byte(entry.Content))
	computed := "sha256:" + hex.EncodeToString(hash[:])
	if computed != entry.Metadata.IntegritySHA256 {
		return ErrPromptIntegrity
	}
	return nil
}

// MarshalJSON implements json.Marshaler for inspection.
func (r *PromptRegistry) MarshalJSON() ([]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return json.Marshal(r.prompts)
}
