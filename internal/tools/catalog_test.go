package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/memory"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/policy"
)

type fakeMemory struct{}

func (fakeMemory) Search(context.Context, string, int) ([]memory.Item, error) {
	return []memory.Item{{ID: 1, Content: "owner prefers Arabic", Source: "test", Created: "2026-10-22"}}, nil
}
func (fakeMemory) Remember(context.Context, string) (int64, error) { return 7, nil }

func newCatalogRegistry(t *testing.T) (*Registry, *Workspace) {
	t.Helper()
	root := t.TempDir()
	workspace := NewWorkspace(filepath.Join(root, "ws"))
	if err := os.MkdirAll(workspace.Root, 0o700); err != nil {
		t.Fatal(err)
	}
	registry := NewRegistry(nil)
	if err := RegisterWorkspace(registry, workspace, nil); err != nil {
		t.Fatal(err)
	}
	if err := RegisterMemory(registry, fakeMemory{}); err != nil {
		t.Fatal(err)
	}
	return registry, workspace
}

func TestCatalogCoversEveryTool(t *testing.T) {
	registry, _ := newCatalogRegistry(t)
	catalog, err := registry.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog) < 7 {
		t.Fatalf("catalog = %d tools", len(catalog))
	}
	seen := map[string]bool{}
	for _, entry := range catalog {
		if seen[entry.Name] {
			t.Fatalf("duplicate %s", entry.Name)
		}
		seen[entry.Name] = true
		if len(entry.Description) < 8 || len(entry.Description) > 1024 {
			t.Fatalf("%s description length %d", entry.Name, len(entry.Description))
		}
		var probe struct {
			Type       string                    `json:"type"`
			Properties map[string]map[string]any `json:"properties"`
		}
		if err := json.Unmarshal(entry.Parameters, &probe); err != nil {
			t.Fatalf("%s schema: %v", entry.Name, err)
		}
		if probe.Type != "object" || len(probe.Properties) == 0 {
			t.Fatalf("%s schema shape: %+v", entry.Name, probe)
		}
		for name, property := range probe.Properties {
			if kind, _ := property["type"].(string); kind == "" {
				t.Fatalf("%s.%s has no type", entry.Name, name)
			}
		}
	}
	for _, expected := range []string{"workspace.read", "workspace.write", "workspace.patch", "workspace.search", "memory.search", "memory.remember"} {
		if !seen[expected] {
			t.Fatalf("missing %s", expected)
		}
	}
}

func TestCatalogMarksApprovalOnlyForWrites(t *testing.T) {
	registry, _ := newCatalogRegistry(t)
	catalog, err := registry.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range catalog {
		wantApproval := strings.HasPrefix(entry.Name, "workspace.") &&
			entry.Name != "workspace.list" && entry.Name != "workspace.read" && entry.Name != "workspace.search"
		if entry.Approval != wantApproval {
			t.Fatalf("%s approval = %v, want %v", entry.Name, entry.Approval, wantApproval)
		}
	}
}

func TestCatalogRejectsIncompleteDeclarations(t *testing.T) {
	cases := []policy.Tool{
		{Name: "x.one", Version: "1", Modes: []policy.Mode{policy.Read}, Timeout: time.Second, OutputLimit: 1024,
			InputSchema: []byte(`{"type":"object","properties":{"a":{"type":"string"}}}`)},
		{Name: "x.two", Version: "1", Description: "has a description", Modes: []policy.Mode{policy.Read}, Timeout: time.Second, OutputLimit: 1024},
		{Name: "x.three", Version: "1", Description: "bad schema", Modes: []policy.Mode{policy.Read}, Timeout: time.Second, OutputLimit: 1024,
			InputSchema: []byte(`[]`)},
		{Name: "x.four", Version: "1", Description: "array schema", Modes: []policy.Mode{policy.Read}, Timeout: time.Second, OutputLimit: 1024,
			InputSchema: []byte(`{"type":"array"}`)},
		{Name: "x.five", Version: "1", Description: "required drift", Modes: []policy.Mode{policy.Read}, Timeout: time.Second, OutputLimit: 1024,
			InputSchema: []byte(`{"type":"object","properties":{},"required":["missing"]}`)},
	}
	for _, spec := range cases {
		registry := NewRegistry(nil)
		if err := registry.Register(Registered{Spec: spec, Run: func(context.Context, json.RawMessage) (any, error) { return nil, nil }}); err != nil {
			t.Fatalf("register %s: %v", spec.Name, err)
		}
		if _, err := registry.Catalog(); err == nil {
			t.Fatalf("%s accepted by catalog", spec.Name)
		}
	}
}

// TestSchemaMatchesHandlers feeds each declared schema's required properties to
// the real handler, so a model-facing schema can never drift from the code that
// executes it.
func TestSchemaMatchesHandlers(t *testing.T) {
	registry, workspace := newCatalogRegistry(t)
	if err := os.WriteFile(filepath.Join(workspace.Root, "seed.md"), []byte("hello world"), 0o600); err != nil {
		t.Fatal(err)
	}
	values := map[string]any{
		"path":    "seed.md",
		"query":   "hello",
		"content": "new content",
		"old":     "hello",
		"new":     "bye",
		"limit":   3,
	}
	catalog, err := registry.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range catalog {
		if entry.Name == "workspace.move_to_trash" {
			continue // destructive by design; covered by its own tests
		}
		var probe struct {
			Properties map[string]map[string]any `json:"properties"`
			Required   []string                  `json:"required"`
		}
		if err := json.Unmarshal(entry.Parameters, &probe); err != nil {
			t.Fatal(err)
		}
		arguments := map[string]any{}
		for _, name := range probe.Required {
			value, ok := values[name]
			if !ok {
				t.Fatalf("%s requires %q but the test has no sample value", entry.Name, name)
			}
			if entry.Name == "workspace.mkdir" && name == "path" {
				value = "newdir"
			}
			arguments[name] = value
		}
		encoded, err := json.Marshal(arguments)
		if err != nil {
			t.Fatal(err)
		}
		registry.mu.RLock()
		handler := registry.tools[entry.Name].Run
		registry.mu.RUnlock()
		result, err := handler(context.Background(), encoded)
		if err != nil {
			t.Fatalf("%s rejected schema-shaped arguments %s: %v", entry.Name, encoded, err)
		}
		if result == nil {
			t.Fatalf("%s returned no result", entry.Name)
		}
	}
}

func TestCatalogDoesNotExposeProcessRunByDefault(t *testing.T) {
	registry, workspace := newCatalogRegistry(t)
	runner := &ProcessRunner{Workspace: workspace.Root, Profiles: DefaultProfiles(), MaxOutput: 4096}
	withProcess := NewRegistry(nil)
	if err := RegisterWorkspace(withProcess, workspace, runner); err != nil {
		t.Fatal(err)
	}
	catalog, err := withProcess.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range catalog {
		if entry.Name == "process.run" {
			found = true
			if !entry.Approval {
				t.Fatal("process.run is not approval gated")
			}
		}
	}
	if !found {
		t.Fatal("process.run missing when a runner is configured")
	}
	if _, err := registry.Catalog(); err != nil {
		t.Fatalf("baseline catalog: %v", err)
	}
}

func TestMemoryToolsRejectBadArguments(t *testing.T) {
	registry, _ := newCatalogRegistry(t)
	cases := map[string]string{
		"memory.search":   `{"query":""}`,
		"memory.remember": `{"content":"   "}`,
		"memory.search#2": `not json`,
	}
	for key, arguments := range cases {
		name := strings.Split(key, "#")[0]
		registry.mu.RLock()
		handler := registry.tools[name].Run
		registry.mu.RUnlock()
		if _, err := handler(context.Background(), json.RawMessage(arguments)); err == nil {
			t.Fatalf("%s accepted %s", name, arguments)
		}
	}
}
