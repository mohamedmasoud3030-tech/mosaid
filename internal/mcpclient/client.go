package mcpclient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/approval"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/audit"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/policy"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/skills"
)

type managedSession struct {
	client  *mcp.Client
	session *mcp.ClientSession
	tools   map[string]*mcp.Tool
}

type Manager struct {
	mu        sync.Mutex
	configs   map[string]ServerConfig
	sessions  map[string]*managedSession
	Approvals *approval.Manager
	Audit     *audit.Logger
}

func NewManager(approvals *approval.Manager, auditLogger *audit.Logger) *Manager {
	return &Manager{configs: map[string]ServerConfig{}, sessions: map[string]*managedSession{}, Approvals: approvals, Audit: auditLogger}
}

func (m *Manager) Register(ctx context.Context, config ServerConfig) error {
	if err := config.Validate(ctx); err != nil {
		return err
	}
	config = cloneConfig(config)
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.configs[config.ID]; exists {
		return fmt.Errorf("%w: duplicate server id", ErrInvalidConfig)
	}
	m.configs[config.ID] = config
	return nil
}

func (m *Manager) InvokeMCP(ctx context.Context, call skills.MCPCall) (any, error) {
	m.mu.Lock()
	config, exists := m.configs[call.ServerID]
	m.mu.Unlock()
	if !exists {
		return nil, fmt.Errorf("%w: server not registered", ErrInvalidConfig)
	}
	if !contains(config.ToolAllowlist, call.Tool) {
		_ = m.record(ctx, call.UserID, call.Resource, "denied_unallowlisted")
		return nil, ErrToolDenied
	}
	if len(call.AllowedNetwork) != 0 && !isSubset(call.AllowedNetwork, config.NetworkAllowlist) {
		_ = m.record(ctx, call.UserID, call.Resource, "denied_network_scope")
		return nil, fmt.Errorf("%w: skill attempted to widen network scope", ErrPolicyDenied)
	}
	if len(call.Arguments) == 0 || len(call.Arguments) > 1024*1024 || !json.Valid(call.Arguments) {
		return nil, errors.New("MCP arguments must be bounded JSON")
	}
	var arguments map[string]any
	decoder := json.NewDecoder(bytes.NewReader(call.Arguments))
	decoder.UseNumber()
	if err := decoder.Decode(&arguments); err != nil || arguments == nil {
		return nil, errors.New("MCP arguments must be a JSON object")
	}
	if err := m.authorize(ctx, config, call); err != nil {
		_ = m.record(ctx, call.UserID, call.Resource, "denied_policy")
		return nil, err
	}
	if err := m.record(ctx, call.UserID, call.Resource, "allowed"); err != nil {
		return nil, fmt.Errorf("MCP audit: %w", err)
	}

	invocationCtx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()
	var lastErr error
	for attempt := 0; attempt < config.MaxAttempts; attempt++ {
		session, err := m.getSession(invocationCtx, config)
		if err != nil {
			lastErr = err
			_ = m.Reset(config.ID)
			continue
		}
		if _, available := session.tools[call.Tool]; !available {
			return nil, ErrToolDenied
		}
		result, err := session.session.CallTool(invocationCtx, &mcp.CallToolParams{Name: call.Tool, Arguments: arguments})
		if err != nil {
			lastErr = err
			_ = m.Reset(config.ID)
			continue
		}
		if result.IsError {
			return nil, fmt.Errorf("MCP tool error: %w", result.GetError())
		}
		encoded, err := json.Marshal(result)
		if err != nil {
			return nil, err
		}
		if int64(len(encoded)) > config.OutputLimit {
			_ = m.Reset(config.ID)
			return nil, ErrOutputLimit
		}
		var output any
		if err = json.Unmarshal(encoded, &output); err != nil {
			return nil, err
		}
		return output, nil
	}
	if lastErr == nil {
		lastErr = errors.New("MCP invocation failed")
	}
	_ = m.record(context.WithoutCancel(ctx), call.UserID, call.Resource, "transport_error")
	return nil, lastErr
}

func (m *Manager) authorize(ctx context.Context, config ServerConfig, call skills.MCPCall) error {
	spec := config.Policies[call.Tool]
	mode := call.Mode
	if mode == "" {
		mode = policy.Read
	}
	decision := policy.Evaluate(spec, mode)
	if decision.Allowed {
		return nil
	}
	if !decision.NeedsApproval {
		return fmt.Errorf("%w: %s", ErrPolicyDenied, decision.Reason)
	}
	if call.ApprovalToken == "" {
		return skills.ErrApprovalRequired
	}
	if m.Approvals == nil {
		return fmt.Errorf("%w: approval service unavailable", ErrPolicyDenied)
	}
	hash := sha256.Sum256(call.Arguments)
	if err := m.Approvals.Authorize(ctx, call.ApprovalToken, call.UserID, spec.Name, hex.EncodeToString(hash[:]), call.Resource); err != nil {
		return fmt.Errorf("%w: approval rejected", ErrPolicyDenied)
	}
	return nil
}

func (m *Manager) getSession(ctx context.Context, config ServerConfig) (*managedSession, error) {
	m.mu.Lock()
	if session := m.sessions[config.ID]; session != nil {
		m.mu.Unlock()
		return session, nil
	}
	m.mu.Unlock()

	session, err := connect(ctx, config)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	if existing := m.sessions[config.ID]; existing != nil {
		m.mu.Unlock()
		_ = session.session.Close()
		return existing, nil
	}
	m.sessions[config.ID] = session
	m.mu.Unlock()
	return session, nil
}

func connect(ctx context.Context, config ServerConfig) (*managedSession, error) {
	client := mcp.NewClient(&mcp.Implementation{Name: "mosaid", Title: "Mosaid", Version: "0.1.0"}, nil)
	var transport mcp.Transport
	switch config.Transport {
	case Stdio:
		actual, err := fileSHA256(config.Stdio.Executable)
		if err != nil || !strings.EqualFold(actual, config.Stdio.ChecksumSHA256) {
			return nil, fmt.Errorf("%w: executable changed before launch", ErrInvalidConfig)
		}
		command := exec.CommandContext(ctx, config.Stdio.Executable, config.Stdio.Arguments...)
		command.Dir = config.Stdio.WorkingDir
		command.Env = filteredEnvironment(config)
		transport = &mcp.CommandTransport{Command: command, TerminateDuration: config.Timeout}
	case StreamableHTTP:
		transport = &mcp.StreamableClientTransport{
			Endpoint:             config.HTTP.Endpoint,
			HTTPClient:           pinnedHTTPClient(config.HTTP),
			MaxRetries:           -1,
			DisableStandaloneSSE: config.HTTP.DisableStandaloneSSE,
		}
	default:
		return nil, ErrInvalidConfig
	}
	session, err := client.Connect(ctx, transport, nil)
	if err != nil {
		return nil, err
	}
	tools, err := discoverAllowedTools(ctx, session, config.ToolAllowlist)
	if err != nil {
		_ = session.Close()
		return nil, err
	}
	return &managedSession{client: client, session: session, tools: tools}, nil
}

func discoverAllowedTools(ctx context.Context, session *mcp.ClientSession, allowlist []string) (map[string]*mcp.Tool, error) {
	allowed := make(map[string]struct{}, len(allowlist))
	for _, name := range allowlist {
		allowed[name] = struct{}{}
	}
	found := map[string]*mcp.Tool{}
	cursor := ""
	for page := 0; page < 8; page++ {
		result, err := session.ListTools(ctx, &mcp.ListToolsParams{Cursor: cursor})
		if err != nil {
			return nil, err
		}
		if len(result.Tools) > 256 {
			return nil, fmt.Errorf("%w: excessive tool list", ErrSchemaRejected)
		}
		for _, tool := range result.Tools {
			if tool == nil || !mcpIDPattern.MatchString(tool.Name) {
				return nil, fmt.Errorf("%w: malformed tool identity", ErrSchemaRejected)
			}
			if _, ok := allowed[tool.Name]; !ok {
				continue
			}
			if _, duplicate := found[tool.Name]; duplicate {
				return nil, fmt.Errorf("%w: duplicate tool identity", ErrSchemaRejected)
			}
			if err := validateToolSchema(tool.InputSchema); err != nil {
				return nil, err
			}
			found[tool.Name] = tool
		}
		cursor = result.NextCursor
		if cursor == "" {
			break
		}
		if page == 7 {
			return nil, fmt.Errorf("%w: pagination limit", ErrSchemaRejected)
		}
	}
	for name := range allowed {
		if _, ok := found[name]; !ok {
			return nil, fmt.Errorf("%w: allowlisted tool %q missing", ErrToolDenied, name)
		}
	}
	return found, nil
}

func (m *Manager) Reset(serverID string) error {
	m.mu.Lock()
	session := m.sessions[serverID]
	delete(m.sessions, serverID)
	m.mu.Unlock()
	if session != nil {
		err := session.session.Close()
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return nil
		}
		return err
	}
	return nil
}

func (m *Manager) Close() error {
	m.mu.Lock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	sort.Strings(ids)
	var combined error
	for _, id := range ids {
		combined = errors.Join(combined, m.Reset(id))
	}
	return combined
}

func (m *Manager) record(ctx context.Context, userID int64, resource, decision string) error {
	if m.Audit == nil {
		return nil
	}
	_, err := m.Audit.Append(ctx, audit.Entry{Kind: "mcp_call", UserID: userID, Resource: resource, Decision: decision})
	return err
}

func filteredEnvironment(config ServerConfig) []string {
	environment := []string{
		"PATH=" + filepath.Dir(config.Stdio.Executable),
		"HOME=" + config.Stdio.WorkingDir,
		"TMPDIR=" + config.Stdio.WorkingDir,
		"LANG=C.UTF-8",
	}
	keys := make([]string, 0, len(config.Stdio.Environment))
	for key := range config.Stdio.Environment {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		environment = append(environment, key+"="+config.Stdio.Environment[key])
	}
	return environment
}

func pinnedHTTPClient(config *HTTPConfig) *http.Client {
	base := config.Client
	if base == nil {
		base = &http.Client{}
	}
	clone := *base
	transport := clone.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	clone.Transport = pinningRoundTripper{next: transport, identity: strings.ToLower(config.IdentitySHA256)}
	clone.CheckRedirect = func(*http.Request, []*http.Request) error { return errors.New("MCP redirects are disabled") }
	return &clone
}

type pinningRoundTripper struct {
	next     http.RoundTripper
	identity string
}

func (p pinningRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	response, err := p.next.RoundTrip(request)
	if err != nil {
		return nil, err
	}
	if response.TLS == nil || len(response.TLS.PeerCertificates) == 0 {
		response.Body.Close()
		return nil, errors.New("MCP TLS identity unavailable")
	}
	hash := sha256.Sum256(response.TLS.PeerCertificates[0].Raw)
	if !strings.EqualFold(hex.EncodeToString(hash[:]), p.identity) {
		response.Body.Close()
		return nil, errors.New("MCP TLS identity mismatch")
	}
	return response, nil
}

func cloneConfig(config ServerConfig) ServerConfig {
	config.ToolAllowlist = append([]string(nil), config.ToolAllowlist...)
	config.NetworkAllowlist = append([]string(nil), config.NetworkAllowlist...)
	config.Policies = clonePolicies(config.Policies)
	if config.Stdio != nil {
		stdio := *config.Stdio
		stdio.Arguments = append([]string(nil), stdio.Arguments...)
		stdio.Environment = make(map[string]string, len(config.Stdio.Environment))
		for key, value := range config.Stdio.Environment {
			stdio.Environment[key] = value
		}
		config.Stdio = &stdio
	}
	if config.HTTP != nil {
		httpConfig := *config.HTTP
		config.HTTP = &httpConfig
	}
	return config
}

func clonePolicies(input map[string]policy.Tool) map[string]policy.Tool {
	output := make(map[string]policy.Tool, len(input))
	for name, spec := range input {
		spec.Modes = append([]policy.Mode(nil), spec.Modes...)
		spec.PathScope = append([]string(nil), spec.PathScope...)
		spec.NetworkScope = append([]string(nil), spec.NetworkScope...)
		spec.InputSchema = append([]byte(nil), spec.InputSchema...)
		output[name] = spec
	}
	return output
}

func containsMode(values []policy.Mode, wanted policy.Mode) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func isSubset(requested, allowed []string) bool {
	for _, value := range requested {
		if !contains(allowed, value) {
			return false
		}
	}
	return true
}
