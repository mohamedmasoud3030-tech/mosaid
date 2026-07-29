package skills

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/policy"
)

var (
	ErrNotFound         = errors.New("skill not found")
	ErrApprovalRequired = errors.New("skill approval is required")
	ErrRuntimeMissing   = errors.New("skill runtime is unavailable")
)

type ToolCall struct {
	Name          string
	Arguments     json.RawMessage
	Mode          policy.Mode
	UserID        int64
	Resource      string
	ApprovalToken string
}

type ToolInvoker interface {
	InvokeTool(context.Context, ToolCall) (any, error)
}

type MCPCall struct {
	ServerID       string
	Tool           string
	Arguments      json.RawMessage
	Mode           policy.Mode
	UserID         int64
	Resource       string
	ApprovalToken  string
	AllowedNetwork []string
}

type MCPInvoker interface {
	InvokeMCP(context.Context, MCPCall) (any, error)
}

type ExecutionRequest struct {
	ID            string
	Version       string
	Input         json.RawMessage
	UserID        int64
	Resource      string
	ApprovalToken string
}

type ExecutionResult struct {
	Manifest Manifest
	Value    any
}

type BuiltinHandler func(context.Context, *SkillContext, json.RawMessage) (any, error)

type SkillContext struct {
	manifest      Manifest
	tools         ToolInvoker
	userID        int64
	resource      string
	approvalToken string
}

func (c *SkillContext) CallTool(ctx context.Context, name string, mode policy.Mode, arguments json.RawMessage) (any, error) {
	if c.tools == nil {
		return nil, ErrRuntimeMissing
	}
	if !contains(c.manifest.RequiredTools, name) || !containsMode(c.manifest.RequiredPermissions, mode) {
		return nil, fmt.Errorf("%w: skill attempted to widen tool or permission scope", ErrInvalidManifest)
	}
	return c.tools.InvokeTool(ctx, ToolCall{Name: name, Arguments: arguments, Mode: mode, UserID: c.userID, Resource: c.resource, ApprovalToken: c.approvalToken})
}

type Registry struct {
	mu        sync.RWMutex
	manifests map[string]map[string]Manifest
	builtins  map[string]BuiltinHandler
	Tools     ToolInvoker
	MCP       MCPInvoker
}

func NewRegistry(tools ToolInvoker) *Registry {
	return &Registry{manifests: map[string]map[string]Manifest{}, builtins: map[string]BuiltinHandler{}, Tools: tools}
}

func (r *Registry) Add(manifest Manifest) error {
	if err := manifest.Validate(Capabilities{}); err != nil {
		return err
	}
	if err := manifest.VerifyIntegrity(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	versions := r.manifests[manifest.ID]
	if versions == nil {
		versions = map[string]Manifest{}
		r.manifests[manifest.ID] = versions
	}
	if _, exists := versions[manifest.Version]; exists {
		return ErrConflict
	}
	versions[manifest.Version] = manifest
	return nil
}

func (r *Registry) RegisterBuiltin(id, version string, handler BuiltinHandler) error {
	if handler == nil {
		return errors.New("builtin handler required")
	}
	key := id + "@" + version
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.builtins[key]; exists {
		return ErrConflict
	}
	r.builtins[key] = handler
	return nil
}

func (r *Registry) Resolve(id, version string) (Manifest, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	versions := r.manifests[id]
	if len(versions) == 0 {
		return Manifest{}, ErrNotFound
	}
	if version != "" {
		manifest, exists := versions[version]
		if !exists {
			return Manifest{}, ErrNotFound
		}
		return manifest, nil
	}
	ordered := sortedVersions(versions)
	return versions[ordered[0]], nil
}

func (r *Registry) Execute(ctx context.Context, request ExecutionRequest) (ExecutionResult, error) {
	manifest, err := r.Resolve(request.ID, request.Version)
	if err != nil {
		return ExecutionResult{}, err
	}
	if err := manifest.VerifyIntegrity(); err != nil {
		return ExecutionResult{}, err
	}
	if err := ValidateInput(manifest.InputSchema, request.Input); err != nil {
		return ExecutionResult{}, err
	}
	if manifest.ApprovalPolicy == ApprovalAlways && request.ApprovalToken == "" {
		return ExecutionResult{}, ErrApprovalRequired
	}
	executionContext, cancel := context.WithTimeout(ctx, time.Duration(manifest.TimeoutSeconds)*time.Second)
	defer cancel()
	skillContext := &SkillContext{manifest: manifest, tools: r.Tools, userID: request.UserID, resource: request.Resource, approvalToken: request.ApprovalToken}

	var value any
	switch manifest.Type {
	case Declarative:
		outputs := make([]any, 0, len(manifest.Steps))
		for _, step := range manifest.Steps {
			arguments := step.Arguments
			if len(arguments) == 0 {
				arguments = request.Input
			}
			output, callErr := skillContext.CallTool(executionContext, step.Tool, step.Mode, arguments)
			if callErr != nil {
				return ExecutionResult{}, callErr
			}
			outputs = append(outputs, output)
		}
		value = outputs
	case Builtin:
		r.mu.RLock()
		handler := r.builtins[manifest.ID+"@"+manifest.Version]
		r.mu.RUnlock()
		if handler == nil {
			return ExecutionResult{}, ErrRuntimeMissing
		}
		value, err = handler(executionContext, skillContext, request.Input)
		if err != nil {
			return ExecutionResult{}, err
		}
	case MCP:
		if r.MCP == nil || manifest.MCP == nil {
			return ExecutionResult{}, ErrRuntimeMissing
		}
		value, err = r.MCP.InvokeMCP(executionContext, MCPCall{ServerID: manifest.MCP.ServerID, Tool: manifest.MCP.Tool, Arguments: request.Input, Mode: highestPermission(manifest.RequiredPermissions), UserID: request.UserID, Resource: request.Resource, ApprovalToken: request.ApprovalToken, AllowedNetwork: append([]string(nil), manifest.AllowedNetworks...)})
		if err != nil {
			return ExecutionResult{}, err
		}
	default:
		return ExecutionResult{}, ErrRuntimeMissing
	}
	return ExecutionResult{Manifest: manifest, Value: value}, nil
}

func highestPermission(permissions []policy.Mode) policy.Mode {
	for _, permission := range permissions {
		if permission == policy.Publish {
			return policy.Publish
		}
	}
	for _, permission := range permissions {
		if permission == policy.Write {
			return policy.Write
		}
	}
	return policy.Read
}
