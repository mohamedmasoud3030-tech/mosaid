package tools

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/policy"
	"time"
)

// CatalogEntry is the model-facing view of a registered tool. It is derived
// from the same policy.Tool declaration that the execution path enforces, so
// the schema a model sees can never drift from the schema policy applies.
type CatalogEntry struct {
	Name        string
	Description string
	Risk        policy.Risk
	Approval    bool
	Parameters  json.RawMessage
}

// Catalog validates and returns every tool that may be shown to a model.
// Incomplete declarations fail closed: a tool without a description or a JSON
// Schema object is an error rather than an invisible inconsistency.
func (r *Registry) Catalog() ([]CatalogEntry, error) {
	if r == nil {
		return nil, errors.New("tool registry unavailable")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	entries := make([]CatalogEntry, 0, len(r.tools))
	for name, registered := range r.tools {
		spec := registered.Spec
		description := strings.TrimSpace(spec.Description)
		if len(description) < 8 || len(description) > 1024 {
			return nil, fmt.Errorf("tool %q needs a description between 8 and 1024 characters", name)
		}
		parameters, err := validateParameters(spec.InputSchema)
		if err != nil {
			return nil, fmt.Errorf("tool %q: %w", name, err)
		}
		entries = append(entries, CatalogEntry{
			Name:        name,
			Description: description,
			Risk:        spec.Risk,
			Approval:    spec.Approval || spec.Risk == policy.High || spec.Risk == policy.Critical,
			Parameters:  parameters,
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries, nil
}

func validateParameters(schema []byte) (json.RawMessage, error) {
	if len(schema) == 0 {
		return nil, errors.New("missing input schema")
	}
	if !json.Valid(schema) {
		return nil, errors.New("input schema is not valid JSON")
	}
	var probe struct {
		Type       string         `json:"type"`
		Properties map[string]any `json:"properties"`
		Required   []string       `json:"required"`
	}
	if err := json.Unmarshal(schema, &probe); err != nil {
		return nil, errors.New("input schema is not a JSON object")
	}
	if probe.Type != "object" {
		return nil, errors.New(`input schema must declare "type":"object"`)
	}
	if probe.Properties == nil {
		return nil, errors.New("input schema must declare properties")
	}
	for _, name := range probe.Required {
		if _, ok := probe.Properties[name]; !ok {
			return nil, fmt.Errorf("required property %q is not declared", name)
		}
	}
	return json.RawMessage(schema), nil
}

// schema builds a JSON Schema object document for a registered tool.
func schema(properties map[string]map[string]any, required ...string) []byte {
	document := map[string]any{"type": "object", "properties": properties, "additionalProperties": false}
	if len(required) > 0 {
		document["required"] = required
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		panic(err)
	}
	return encoded
}

func stringProperty(description string) map[string]any {
	return map[string]any{"type": "string", "description": description}
}

func integerProperty(description string, minimum, maximum int) map[string]any {
	return map[string]any{"type": "integer", "description": description, "minimum": minimum, "maximum": maximum}
}

const workspaceReadTimeout = 5 * time.Second
