package mcpclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/audit"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/policy"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/skills"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/storage"
)

type echoInput struct {
	Message string `json:"message" jsonschema:"message to echo"`
}

type echoOutput struct {
	Message string `json:"message"`
	Leaked  bool   `json:"leaked"`
}

func newMockMCPServer(mode string) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "mosaid-test-mcp", Version: "1.0.0"}, nil)
	if mode == "malicious-schema" {
		server.AddTool(&mcp.Tool{
			Name: "echo.tool",
			InputSchema: map[string]any{
				"type":                 "object",
				"properties":           map[string]any{},
				"additionalProperties": false,
				"$ref":                 "https://malicious.example/schema.json",
			},
		}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return &mcp.CallToolResult{StructuredContent: map[string]any{"ok": true}}, nil
		})
		return server
	}
	mcp.AddTool(server, &mcp.Tool{Name: "echo.tool"}, func(ctx context.Context, _ *mcp.CallToolRequest, input echoInput) (*mcp.CallToolResult, echoOutput, error) {
		switch mode {
		case "timeout":
			select {
			case <-ctx.Done():
				return nil, echoOutput{}, ctx.Err()
			case <-time.After(5 * time.Second):
			}
		case "oversized":
			return nil, echoOutput{Message: strings.Repeat("x", 32*1024)}, nil
		}
		return nil, echoOutput{Message: input.Message, Leaked: os.Getenv("MOSAID_PARENT_SECRET") != ""}, nil
	})
	server.AddTool(&mcp.Tool{Name: "admin.tool", InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "additionalProperties": false}}, func(context.Context, *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{StructuredContent: map[string]any{"admin": true}}, nil
	})
	return server
}

func TestMCPHelperProcess(t *testing.T) {
	if os.Getenv("MOSAID_MCP_HELPER") != "1" {
		return
	}
	mode := os.Getenv("MOSAID_MCP_MODE")
	err := newMockMCPServer(mode).Run(context.Background(), &mcp.StdioTransport{})
	if err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

func safePolicy(serverID string, risk policy.Risk, modes ...policy.Mode) policy.Tool {
	return policy.Tool{
		Name:        "mcp." + serverID + ".echo.tool",
		Version:     "1.0.0",
		Risk:        risk,
		Modes:       modes,
		Timeout:     time.Second,
		OutputLimit: 1024 * 1024,
		Idempotency: policy.Idempotent,
	}
}

func stdioConfig(t *testing.T, mode string) ServerConfig {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	checksum, err := fileSHA256(executable)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	return ServerConfig{
		ID:            "mock.server",
		Source:        "https://example.invalid/mcp/mock@fixed-commit",
		Version:       "1.0.0",
		Transport:     Stdio,
		ToolAllowlist: []string{"echo.tool"},
		Policies: map[string]policy.Tool{
			"echo.tool": safePolicy("mock.server", policy.Safe, policy.Read),
		},
		WorkingRoot: root,
		Timeout:     time.Second,
		OutputLimit: 1024 * 1024,
		MaxAttempts: 1,
		Stdio: &StdioConfig{
			Executable:     executable,
			Arguments:      []string{"-test.run=^TestMCPHelperProcess$"},
			ChecksumSHA256: checksum,
			WorkingDir:     root,
			Environment: map[string]string{
				"MOSAID_MCP_HELPER": "1",
				"MOSAID_MCP_MODE":   mode,
			},
		},
	}
}

func invokeEcho(manager *Manager, mode policy.Mode) (any, error) {
	return manager.InvokeMCP(context.Background(), skills.MCPCall{
		ServerID:  "mock.server",
		Tool:      "echo.tool",
		Arguments: json.RawMessage(`{"message":"hello"}`),
		Mode:      mode,
		UserID:    7,
		Resource:  "test-task",
	})
}

func TestStdioUsesOfficialSDKAndFiltersEnvironment(t *testing.T) {
	t.Setenv("MOSAID_PARENT_SECRET", "must-not-reach-child")
	manager := NewManager(nil, nil)
	defer manager.Close()
	if err := manager.Register(context.Background(), stdioConfig(t, "normal")); err != nil {
		t.Fatal(err)
	}
	output, err := invokeEcho(manager, policy.Read)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(output)
	text := string(encoded)
	if !strings.Contains(text, "hello") || !strings.Contains(text, `"leaked":false`) || strings.Contains(text, "must-not-reach-child") {
		t.Fatalf("unexpected output: %s", text)
	}
}

func TestStreamableHTTPUsesPinnedTLSIdentity(t *testing.T) {
	server := newMockMCPServer("normal")
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true, MaxRequestBodyBytes: 1 << 20, PropagateRequestCancellation: true})
	httpServer := httptest.NewTLSServer(handler)
	defer httpServer.Close()
	certificate := httpServer.Certificate()
	hash := sha256.Sum256(certificate.Raw)
	parsed, _ := url.Parse(httpServer.URL)
	config := ServerConfig{
		ID:               "mock.server",
		Source:           "https://example.invalid/mcp/mock@fixed-commit",
		Version:          "1.0.0",
		Transport:        StreamableHTTP,
		ToolAllowlist:    []string{"echo.tool"},
		Policies:         map[string]policy.Tool{"echo.tool": safePolicy("mock.server", policy.Safe, policy.Read)},
		NetworkAllowlist: []string{parsed.Hostname()},
		Timeout:          time.Second,
		OutputLimit:      1024 * 1024,
		MaxAttempts:      1,
		HTTP: &HTTPConfig{
			Endpoint:             httpServer.URL,
			IdentitySHA256:       hex.EncodeToString(hash[:]),
			Client:               httpServer.Client(),
			AllowLoopback:        true,
			DisableStandaloneSSE: true,
		},
	}
	manager := NewManager(nil, nil)
	defer manager.Close()
	if err := manager.Register(context.Background(), config); err != nil {
		t.Fatal(err)
	}
	output, err := invokeEcho(manager, policy.Read)
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := json.Marshal(output)
	if !strings.Contains(string(encoded), "hello") {
		t.Fatalf("output=%s", encoded)
	}

	config.ID = "wrong.identity"
	config.Policies = map[string]policy.Tool{"echo.tool": safePolicy(config.ID, policy.Safe, policy.Read)}
	config.HTTP.IdentitySHA256 = strings.Repeat("0", 64)
	badManager := NewManager(nil, nil)
	if err = badManager.Register(context.Background(), config); err != nil {
		t.Fatal(err)
	}
	_, err = badManager.InvokeMCP(context.Background(), skills.MCPCall{ServerID: config.ID, Tool: "echo.tool", Arguments: json.RawMessage(`{"message":"x"}`), Mode: policy.Read})
	if err == nil || !strings.Contains(err.Error(), "identity mismatch") {
		t.Fatalf("identity err=%v", err)
	}
}

func TestUnapprovedToolRejectedWithoutDiscovery(t *testing.T) {
	manager := NewManager(nil, nil)
	if err := manager.Register(context.Background(), stdioConfig(t, "normal")); err != nil {
		t.Fatal(err)
	}
	_, err := manager.InvokeMCP(context.Background(), skills.MCPCall{ServerID: "mock.server", Tool: "admin.tool", Arguments: json.RawMessage(`{}`), Mode: policy.Admin})
	if !errors.Is(err, ErrToolDenied) {
		t.Fatalf("err=%v", err)
	}
}

func TestMaliciousSchemaRejected(t *testing.T) {
	manager := NewManager(nil, nil)
	defer manager.Close()
	if err := manager.Register(context.Background(), stdioConfig(t, "malicious-schema")); err != nil {
		t.Fatal(err)
	}
	_, err := invokeEcho(manager, policy.Read)
	if !errors.Is(err, ErrSchemaRejected) {
		t.Fatalf("err=%v", err)
	}
}

func TestTimeoutAndOversizedOutput(t *testing.T) {
	t.Run("timeout", func(t *testing.T) {
		config := stdioConfig(t, "timeout")
		config.Timeout = 50 * time.Millisecond
		manager := NewManager(nil, nil)
		defer manager.Close()
		if err := manager.Register(context.Background(), config); err != nil {
			t.Fatal(err)
		}
		started := time.Now()
		_, err := invokeEcho(manager, policy.Read)
		if err == nil || time.Since(started) > time.Second {
			t.Fatalf("err=%v duration=%s", err, time.Since(started))
		}
	})
	t.Run("oversized", func(t *testing.T) {
		config := stdioConfig(t, "oversized")
		config.OutputLimit = 512
		manager := NewManager(nil, nil)
		defer manager.Close()
		if err := manager.Register(context.Background(), config); err != nil {
			t.Fatal(err)
		}
		_, err := invokeEcho(manager, policy.Read)
		if !errors.Is(err, ErrOutputLimit) {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestPathEscapeChecksumAndLauncherRejected(t *testing.T) {
	t.Run("path escape", func(t *testing.T) {
		config := stdioConfig(t, "normal")
		config.Stdio.WorkingDir = t.TempDir()
		if err := config.Validate(context.Background()); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("checksum", func(t *testing.T) {
		config := stdioConfig(t, "normal")
		config.Stdio.ChecksumSHA256 = strings.Repeat("0", 64)
		if err := config.Validate(context.Background()); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("launcher", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "npx")
		if err := os.WriteFile(path, []byte("not executed"), 0o700); err != nil {
			t.Fatal(err)
		}
		checksum, _ := fileSHA256(path)
		config := stdioConfig(t, "normal")
		config.WorkingRoot = root
		config.Stdio.Executable = path
		config.Stdio.WorkingDir = root
		config.Stdio.ChecksumSHA256 = checksum
		if err := config.Validate(context.Background()); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestPolicyDenialBeforeMCPCall(t *testing.T) {
	config := stdioConfig(t, "normal")
	manager := NewManager(nil, nil)
	if err := manager.Register(context.Background(), config); err != nil {
		t.Fatal(err)
	}
	_, err := invokeEcho(manager, policy.Write)
	if !errors.Is(err, ErrPolicyDenied) {
		t.Fatalf("err=%v", err)
	}
}

func TestMCPInvocationIsHashChainAudited(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	logger := &audit.Logger{DB: db.SQL()}
	manager := NewManager(nil, logger)
	defer manager.Close()
	if err = manager.Register(context.Background(), stdioConfig(t, "normal")); err != nil {
		t.Fatal(err)
	}
	if _, err = invokeEcho(manager, policy.Read); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.SQL().QueryRow(`SELECT count(*) FROM audit_entries WHERE kind='mcp_call' AND decision='allowed'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	if err = logger.Verify(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSessionRestartIsExplicitAndRecoverable(t *testing.T) {
	manager := NewManager(nil, nil)
	defer manager.Close()
	if err := manager.Register(context.Background(), stdioConfig(t, "normal")); err != nil {
		t.Fatal(err)
	}
	if _, err := invokeEcho(manager, policy.Read); err != nil {
		t.Fatal(err)
	}
	if err := manager.Reset("mock.server"); err != nil {
		t.Fatal(err)
	}
	if _, err := invokeEcho(manager, policy.Read); err != nil {
		t.Fatal(err)
	}
}
