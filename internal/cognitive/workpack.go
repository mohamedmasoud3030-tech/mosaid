package cognitive

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"
)

var (
	ErrInvalidWorkPack  = errors.New("invalid work pack")
	ErrWorkPackNotFound = errors.New("work pack not found")
)

var workPackIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{1,127}$`)

// WorkPackStatus represents the completeness status of a work pack.
type WorkPackStatus string

const (
	WorkPackDraft      WorkPackStatus = "draft"
	WorkPackBuilding   WorkPackStatus = "building"
	WorkPackComplete   WorkPackStatus = "complete"
	WorkPackDeprecated WorkPackStatus = "deprecated"
)

// WorkPack represents a pre-defined, tested, beginner-friendly task template.
type WorkPack struct {
	ID               string          `json:"id"`
	Name             string          `json:"name"`
	Description      string          `json:"description"`
	Version          string          `json:"version"`
	Status           WorkPackStatus  `json:"status"`
	Priority         int             `json:"priority"` // 1 = highest
	Category         string          `json:"category"`
	IntakeSchema     json.RawMessage `json:"intake_schema"` // JSON Schema for inputs
	Workflow         []WorkflowStep  `json:"workflow"`      // Ordered steps
	RequiredTools    []string        `json:"required_tools"`
	QualityChecks    []QualityCheck  `json:"quality_checks"`
	DeliveryFormat   string          `json:"delivery_format"` // e.g., "markdown", "json", "file"
	BeginnerGuide    string          `json:"beginner_guide"`  // Step-by-step guide for beginners
	EstimatedMinutes int             `json:"estimated_minutes"`
	CreatedAt        time.Time       `json:"created_at"`
	UpdatedAt        time.Time       `json:"updated_at"`
}

// WorkflowStep represents one step in a work pack's workflow.
type WorkflowStep struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	ToolName    string `json:"tool_name,omitempty"`
	Risk        string `json:"risk"`
	Approval    bool   `json:"approval,omitempty"`
}

// QualityCheck represents an automated quality verification.
type QualityCheck struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Type        string `json:"type"` // "automated", "manual", "llm"
	Criteria    string `json:"criteria"`
}

// CompletenessCheck verifies a work pack has all required components.
func (wp WorkPack) CompletenessCheck() error {
	if wp.ID == "" || !workPackIDPattern.MatchString(wp.ID) {
		return fmt.Errorf("%w: invalid ID", ErrInvalidWorkPack)
	}
	if wp.Name == "" || len(wp.Name) > 256 {
		return fmt.Errorf("%w: name required (max 256 chars)", ErrInvalidWorkPack)
	}
	if wp.Description == "" || len(wp.Description) > 2048 {
		return fmt.Errorf("%w: description required (max 2048 chars)", ErrInvalidWorkPack)
	}
	if wp.Version == "" {
		return fmt.Errorf("%w: version required", ErrInvalidWorkPack)
	}
	if len(wp.IntakeSchema) == 0 || !json.Valid(wp.IntakeSchema) {
		return fmt.Errorf("%w: intake_schema required and must be valid JSON", ErrInvalidWorkPack)
	}
	if len(wp.Workflow) == 0 || len(wp.Workflow) > 32 {
		return fmt.Errorf("%w: workflow must have 1-32 steps", ErrInvalidWorkPack)
	}
	if len(wp.RequiredTools) == 0 {
		return fmt.Errorf("%w: required_tools cannot be empty", ErrInvalidWorkPack)
	}
	if len(wp.QualityChecks) == 0 {
		return fmt.Errorf("%w: quality_checks required", ErrInvalidWorkPack)
	}
	if wp.DeliveryFormat == "" {
		return fmt.Errorf("%w: delivery_format required", ErrInvalidWorkPack)
	}
	if wp.BeginnerGuide == "" || len(wp.BeginnerGuide) < 50 {
		return fmt.Errorf("%w: beginner_guide required (min 50 chars)", ErrInvalidWorkPack)
	}
	return nil
}

// WorkPackRegistry manages registered work packs.
type WorkPackRegistry struct {
	packs map[string]*WorkPack
}

// NewWorkPackRegistry creates a new work pack registry.
func NewWorkPackRegistry() *WorkPackRegistry {
	return &WorkPackRegistry{
		packs: make(map[string]*WorkPack),
	}
}

// Register adds a work pack to the registry.
func (r *WorkPackRegistry) Register(wp *WorkPack) error {
	if err := wp.CompletenessCheck(); err != nil {
		return err
	}
	if _, exists := r.packs[wp.ID]; exists {
		return fmt.Errorf("work pack %q already registered", wp.ID)
	}
	wp.UpdatedAt = time.Now().UTC()
	if wp.CreatedAt.IsZero() {
		wp.CreatedAt = wp.UpdatedAt
	}
	r.packs[wp.ID] = wp
	return nil
}

// Get returns a work pack by ID.
func (r *WorkPackRegistry) Get(id string) (*WorkPack, error) {
	wp, exists := r.packs[id]
	if !exists {
		return nil, fmt.Errorf("%w: %q", ErrWorkPackNotFound, id)
	}
	return wp, nil
}

// List returns all registered work packs.
func (r *WorkPackRegistry) List() []*WorkPack {
	result := make([]*WorkPack, 0, len(r.packs))
	for _, wp := range r.packs {
		result = append(result, wp)
	}
	return result
}

// ListByCategory returns work packs matching the given category.
func (r *WorkPackRegistry) ListByCategory(category string) []*WorkPack {
	var result []*WorkPack
	for _, wp := range r.packs {
		if wp.Category == category {
			result = append(result, wp)
		}
	}
	return result
}
