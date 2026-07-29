package skills

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/policy"
)

func objectSchema(properties string, required string) json.RawMessage {
	text := `{"type":"object","properties":{` + properties + `},"additionalProperties":false`
	if required != "" {
		text += `,"required":["` + required + `"]`
	}
	return json.RawMessage(text + `}`)
}

func signed(manifest Manifest) Manifest {
	hash, err := manifest.ComputeIntegrity()
	if err != nil {
		panic(err)
	}
	manifest.IntegrityHash = hash
	return manifest
}

func validDeclarative() Manifest {
	return signed(Manifest{
		ID:          "research.basic",
		Version:     "1.0.0",
		Description: "Run an approved bounded research lookup",
		Type:        Declarative,
		InputSchema: objectSchema(`"query":{"type":"string","minLength":1,"maxLength":200}`, "query"),
		Outputs: []Output{{
			Name:        "result",
			Description: "Structured lookup result",
			Schema:      objectSchema(``, ""),
		}},
		RequiredTools:       []string{"workspace.read"},
		RequiredPermissions: []policy.Mode{policy.Read},
		RequiredSecrets:     []string{},
		AllowedNetworks:     []string{},
		TimeoutSeconds:      30,
		ApprovalPolicy:      ApprovalByPolicy,
		Compatibility:       Compatibility{MinCore: "0.1.0", OS: []string{"linux", "android"}, Arch: []string{"amd64", "arm64"}},
		Steps:               []Step{{Tool: "workspace.read", Mode: policy.Read}},
	})
}

func writeManifest(t *testing.T, root, name string, manifest Manifest) string {
	t.Helper()
	dir := filepath.Join(root, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "skill.json")
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func loaderFor(root string) Loader {
	return Loader{Root: root, Capabilities: Capabilities{
		Core: "0.1.0", OS: "linux", Arch: "amd64",
		Tools:    map[string]struct{}{"workspace.read": {}},
		Networks: map[string]struct{}{"api.example.com": {}},
	}}
}

func TestLoaderVerifiesStrictManifestAndIntegrity(t *testing.T) {
	root := t.TempDir()
	manifest := validDeclarative()
	path := writeManifest(t, root, "research", manifest)
	loaded, err := loaderFor(root).LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != manifest.ID || loaded.IntegrityHash != manifest.IntegrityHash {
		t.Fatalf("loaded=%+v", loaded)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]any
	if err = json.Unmarshal(data, &generic); err != nil {
		t.Fatal(err)
	}
	generic["description"] = "tampered description"
	tampered, _ := json.Marshal(generic)
	if err = os.WriteFile(path, tampered, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err = loaderFor(root).LoadFile(path); !errors.Is(err, ErrIntegrity) {
		t.Fatalf("err=%v", err)
	}
}

func TestLoaderRejectsUnknownAndDuplicateFields(t *testing.T) {
	root := t.TempDir()
	manifest := validDeclarative()
	path := writeManifest(t, root, "unknown", manifest)
	data, _ := os.ReadFile(path)
	var generic map[string]any
	_ = json.Unmarshal(data, &generic)
	generic["install_command"] = "download-and-run"
	data, _ = json.Marshal(generic)
	_ = os.WriteFile(path, data, 0o600)
	if _, err := loaderFor(root).LoadFile(path); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("unknown field err=%v", err)
	}

	path = writeManifest(t, root, "duplicate", manifest)
	data, _ = os.ReadFile(path)
	data = []byte(`{"id":"research.basic",` + string(data[1:]))
	_ = os.WriteFile(path, data, 0o600)
	if _, err := loaderFor(root).LoadFile(path); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("duplicate field err=%v", err)
	}
}

func TestLoaderRejectsSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	target := writeManifest(t, outside, "outside", validDeclarative())
	link := filepath.Join(root, "skill.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := loaderFor(root).LoadFile(link); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("err=%v", err)
	}
}

func TestMaliciousManifestScopesRejected(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Manifest)
	}{
		{"remote schema", func(m *Manifest) {
			m.InputSchema = json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false,"$ref":"https://evil.example/schema"}`)
		}},
		{"wildcard network", func(m *Manifest) { m.AllowedNetworks = []string{"*.example.com"} }},
		{"local network", func(m *Manifest) { m.AllowedNetworks = []string{"127.0.0.1"} }},
		{"undeclared executable tool", func(m *Manifest) { m.Steps[0].Tool = "process.run" }},
		{"permission widening", func(m *Manifest) { m.Steps[0].Mode = policy.Write }},
		{"core modification permission", func(m *Manifest) { m.RequiredPermissions = []policy.Mode{policy.Admin} }},
		{"secret value not reference", func(m *Manifest) { m.RequiredSecrets = []string{"actual-secret-value"} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			manifest := validDeclarative()
			test.mutate(&manifest)
			manifest = signed(manifest)
			if err := manifest.Validate(Capabilities{}); !errors.Is(err, ErrInvalidManifest) {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func TestCapabilityAndCompatibilityEnforced(t *testing.T) {
	manifest := validDeclarative()
	if err := manifest.Validate(Capabilities{Tools: map[string]struct{}{"workspace.list": {}}}); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("missing tool err=%v", err)
	}
	manifest.Compatibility.MinCore = "9.0.0"
	manifest = signed(manifest)
	if err := manifest.Validate(Capabilities{Core: "0.1.0"}); !errors.Is(err, ErrIncompatible) {
		t.Fatalf("compatibility err=%v", err)
	}
}

func TestPublishManifestRequiresBoundApproval(t *testing.T) {
	manifest := validDeclarative()
	manifest.RequiredPermissions = []policy.Mode{policy.Read, policy.Publish}
	manifest.Steps = append(manifest.Steps, Step{Tool: "workspace.read", Mode: policy.Publish})
	manifest.ApprovalPolicy = ApprovalByPolicy
	manifest = signed(manifest)
	if err := manifest.Validate(Capabilities{}); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("err=%v", err)
	}
	manifest.ApprovalPolicy = ApprovalBound
	manifest = signed(manifest)
	if err := manifest.Validate(Capabilities{}); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryVersionConflictsAndResolution(t *testing.T) {
	registry := NewRegistry(nil)
	first := validDeclarative()
	if err := registry.Add(first); err != nil {
		t.Fatal(err)
	}
	if err := registry.Add(first); !errors.Is(err, ErrConflict) {
		t.Fatalf("err=%v", err)
	}
	newer := first
	newer.Version = "1.2.0"
	newer = signed(newer)
	if err := registry.Add(newer); err != nil {
		t.Fatal(err)
	}
	resolved, err := registry.Resolve(first.ID, "")
	if err != nil || resolved.Version != newer.Version {
		t.Fatalf("resolved=%+v err=%v", resolved, err)
	}
}

type recordingTools struct {
	mu    sync.Mutex
	calls []ToolCall
}

func (r *recordingTools) InvokeTool(_ context.Context, call ToolCall) (any, error) {
	r.mu.Lock()
	r.calls = append(r.calls, call)
	r.mu.Unlock()
	return map[string]any{"ok": true}, nil
}

func TestDeclarativeSkillRoutesThroughToolInvoker(t *testing.T) {
	tools := &recordingTools{}
	registry := NewRegistry(tools)
	manifest := validDeclarative()
	if err := registry.Add(manifest); err != nil {
		t.Fatal(err)
	}
	input := json.RawMessage(`{"query":"bounded research"}`)
	result, err := registry.Execute(context.Background(), ExecutionRequest{ID: manifest.ID, Input: input, UserID: 7, Resource: "task-1"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Manifest.Version != manifest.Version || len(tools.calls) != 1 {
		t.Fatalf("result=%+v calls=%+v", result, tools.calls)
	}
	call := tools.calls[0]
	if call.Name != "workspace.read" || call.Mode != policy.Read || call.UserID != 7 || string(call.Arguments) != string(input) {
		t.Fatalf("call=%+v", call)
	}
}

func TestBuiltinSkillUsesConstrainedContext(t *testing.T) {
	tools := &recordingTools{}
	registry := NewRegistry(tools)
	manifest := validDeclarative()
	manifest.ID = "coding.builtin"
	manifest.Type = Builtin
	manifest.Steps = nil
	manifest = signed(manifest)
	if err := registry.Add(manifest); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterBuiltin(manifest.ID, manifest.Version, func(ctx context.Context, skill *SkillContext, input json.RawMessage) (any, error) {
		return skill.CallTool(ctx, "workspace.read", policy.Read, input)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Execute(context.Background(), ExecutionRequest{ID: manifest.ID, Input: json.RawMessage(`{"query":"file"}`)}); err != nil {
		t.Fatal(err)
	}
	if len(tools.calls) != 1 {
		t.Fatalf("calls=%d", len(tools.calls))
	}
}

func TestInputSchemaRejectsUnknownWrongTypeAndDuplicate(t *testing.T) {
	schema := validDeclarative().InputSchema
	for _, input := range []json.RawMessage{
		json.RawMessage(`{"query":7}`),
		json.RawMessage(`{"query":"ok","extra":true}`),
		json.RawMessage(`{"query":"a","query":"b"}`),
		json.RawMessage(`{}`),
	} {
		if err := ValidateInput(schema, input); !errors.Is(err, ErrInputSchema) {
			t.Fatalf("input=%s err=%v", input, err)
		}
	}
	if err := ValidateInput(schema, json.RawMessage(`{"query":"ok"}`)); err != nil {
		t.Fatal(err)
	}
}

func TestRepositoryExampleManifests(t *testing.T) {
	root := filepath.Clean(filepath.Join("..", "..", "skills"))
	loader := Loader{Root: root, Capabilities: Capabilities{
		Core: "0.1.0", OS: "linux", Arch: "amd64",
		Tools: map[string]struct{}{
			"workspace.list": {}, "workspace.read": {}, "workspace.search": {}, "workspace.write": {}, "workspace.patch": {}, "process.run": {},
			"images.generate": {}, "instagram.prepare": {}, "instagram.preview": {}, "instagram.publish": {},
		},
		Networks: map[string]struct{}{"github.com": {}, "api.github.com": {}, "api.openai.com": {}, "graph.facebook.com": {}},
	}}
	for _, name := range []string{"coding", "research", "image-generation", "social-publishing"} {
		manifest, err := loader.LoadFile(filepath.Join(root, name, "skill.json"))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if manifest.ID != name {
			t.Fatalf("%s id=%s", name, manifest.ID)
		}
	}
}

func TestMCPBackedSkillIsExplicitAndUnavailableFailClosed(t *testing.T) {
	manifest := validDeclarative()
	manifest.ID = "research.mcp"
	manifest.Type = MCP
	manifest.Steps = nil
	manifest.MCP = &MCPBinding{ServerID: "pinned.server", Tool: "search.tool"}
	manifest = signed(manifest)
	registry := NewRegistry(nil)
	if err := registry.Add(manifest); err != nil {
		t.Fatal(err)
	}
	_, err := registry.Execute(context.Background(), ExecutionRequest{ID: manifest.ID, Input: json.RawMessage(`{"query":"test"}`)})
	if !errors.Is(err, ErrRuntimeMissing) {
		t.Fatalf("err=%v", err)
	}
}
