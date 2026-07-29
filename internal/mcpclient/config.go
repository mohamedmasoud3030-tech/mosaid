package mcpclient

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/policy"
)

var (
	ErrInvalidConfig  = errors.New("invalid MCP server configuration")
	ErrToolDenied     = errors.New("MCP tool is not allowlisted")
	ErrSchemaRejected = errors.New("MCP tool schema rejected")
	ErrOutputLimit    = errors.New("MCP output limit exceeded")
	ErrPolicyDenied   = errors.New("MCP invocation denied by policy")
	mcpIDPattern      = regexp.MustCompile(`^[a-z][a-z0-9.-]{1,127}$`)
	envNamePattern    = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,127}$`)
	versionPattern    = regexp.MustCompile(`^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?$`)
)

type Transport string

const (
	Stdio          Transport = "stdio"
	StreamableHTTP Transport = "streamable_http"
)

type StdioConfig struct {
	Executable     string
	Arguments      []string
	ChecksumSHA256 string
	WorkingDir     string
	Environment    map[string]string
}

type HTTPConfig struct {
	Endpoint             string
	IdentitySHA256       string
	Client               *http.Client
	AllowLoopback        bool
	DisableStandaloneSSE bool
}

type ServerConfig struct {
	ID               string
	Source           string
	Version          string
	Transport        Transport
	ToolAllowlist    []string
	Policies         map[string]policy.Tool
	WorkingRoot      string
	NetworkAllowlist []string
	Timeout          time.Duration
	OutputLimit      int64
	MaxAttempts      int
	Stdio            *StdioConfig
	HTTP             *HTTPConfig
}

func (c ServerConfig) Validate(ctx context.Context) error {
	if !mcpIDPattern.MatchString(c.ID) || strings.TrimSpace(c.Source) == "" || !versionPattern.MatchString(c.Version) {
		return fmt.Errorf("%w: pinned id, source, and semantic version required", ErrInvalidConfig)
	}
	if c.Timeout <= 0 || c.Timeout > 10*time.Minute || c.OutputLimit < 1 || c.OutputLimit > 16*1024*1024 || c.MaxAttempts < 1 || c.MaxAttempts > 2 {
		return fmt.Errorf("%w: limits out of bounds", ErrInvalidConfig)
	}
	if len(c.ToolAllowlist) == 0 || len(c.ToolAllowlist) > 128 {
		return fmt.Errorf("%w: bounded tool allowlist required", ErrInvalidConfig)
	}
	seen := map[string]struct{}{}
	for _, tool := range c.ToolAllowlist {
		if !mcpIDPattern.MatchString(tool) {
			return fmt.Errorf("%w: invalid tool name", ErrInvalidConfig)
		}
		if _, exists := seen[tool]; exists {
			return fmt.Errorf("%w: duplicate tool", ErrInvalidConfig)
		}
		seen[tool] = struct{}{}
		spec, exists := c.Policies[tool]
		if !exists || spec.Name != "mcp."+c.ID+"."+tool {
			return fmt.Errorf("%w: every tool needs an exact core policy", ErrInvalidConfig)
		}
		if err := policy.Validate(spec); err != nil {
			return fmt.Errorf("%w: tool policy: %v", ErrInvalidConfig, err)
		}
	}
	for tool := range c.Policies {
		if _, ok := seen[tool]; !ok {
			return fmt.Errorf("%w: policy exists outside allowlist", ErrInvalidConfig)
		}
	}
	if err := validateNetworks(c.NetworkAllowlist); err != nil {
		return err
	}
	switch c.Transport {
	case Stdio:
		if c.Stdio == nil || c.HTTP != nil {
			return fmt.Errorf("%w: exactly one stdio transport required", ErrInvalidConfig)
		}
		return c.validateStdio(ctx)
	case StreamableHTTP:
		if c.HTTP == nil || c.Stdio != nil {
			return fmt.Errorf("%w: exactly one HTTP transport required", ErrInvalidConfig)
		}
		return c.validateHTTP()
	default:
		return fmt.Errorf("%w: unsupported transport", ErrInvalidConfig)
	}
}

func (c ServerConfig) validateStdio(context.Context) error {
	cfg := c.Stdio
	if !filepath.IsAbs(cfg.Executable) || cfg.WorkingDir == "" || c.WorkingRoot == "" {
		return fmt.Errorf("%w: absolute executable and bounded working directory required", ErrInvalidConfig)
	}
	base := strings.ToLower(filepath.Base(cfg.Executable))
	forbidden := map[string]struct{}{"sh": {}, "bash": {}, "zsh": {}, "dash": {}, "npx": {}, "npm": {}, "pnpm": {}, "curl": {}, "wget": {}, "powershell": {}, "cmd.exe": {}}
	if _, denied := forbidden[base]; denied {
		return fmt.Errorf("%w: shell and download launchers are forbidden", ErrInvalidConfig)
	}
	info, err := os.Lstat(cfg.Executable)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode()&0o111 == 0 {
		return fmt.Errorf("%w: executable must be an executable regular non-symlink file", ErrInvalidConfig)
	}
	if !validSHA256(cfg.ChecksumSHA256) {
		return fmt.Errorf("%w: executable checksum required", ErrInvalidConfig)
	}
	actual, err := fileSHA256(cfg.Executable)
	if err != nil || !strings.EqualFold(actual, cfg.ChecksumSHA256) {
		return fmt.Errorf("%w: executable checksum mismatch", ErrInvalidConfig)
	}
	root, err := canonicalDirectory(c.WorkingRoot)
	if err != nil {
		return fmt.Errorf("%w: working root: %v", ErrInvalidConfig, err)
	}
	working, err := canonicalDirectory(cfg.WorkingDir)
	if err != nil {
		return fmt.Errorf("%w: working directory: %v", ErrInvalidConfig, err)
	}
	if !within(root, working) {
		return fmt.Errorf("%w: working directory escapes root", ErrInvalidConfig)
	}
	for _, arg := range cfg.Arguments {
		if arg == "" || len(arg) > 4096 || strings.ContainsRune(arg, 0) {
			return fmt.Errorf("%w: malformed fixed argument", ErrInvalidConfig)
		}
	}
	for key, value := range cfg.Environment {
		if !envNamePattern.MatchString(key) || len(value) > 8192 || strings.ContainsRune(value, 0) {
			return fmt.Errorf("%w: malformed explicit environment", ErrInvalidConfig)
		}
		if key == "LD_PRELOAD" || key == "DYLD_INSERT_LIBRARIES" || key == "GODEBUG" || key == "GOTRACEBACK" {
			return fmt.Errorf("%w: dangerous environment key", ErrInvalidConfig)
		}
	}
	if len(c.NetworkAllowlist) != 0 {
		return fmt.Errorf("%w: stdio subprocess network access cannot be strongly confined; require an empty network allowlist", ErrInvalidConfig)
	}
	return nil
}

func (c ServerConfig) validateHTTP() error {
	cfg := c.HTTP
	parsed, err := url.Parse(cfg.Endpoint)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil || parsed.Fragment != "" {
		return fmt.Errorf("%w: streamable HTTP endpoint must be an HTTPS URL without userinfo or fragment", ErrInvalidConfig)
	}
	if !validSHA256(cfg.IdentitySHA256) {
		return fmt.Errorf("%w: pinned TLS identity required", ErrInvalidConfig)
	}
	host := strings.ToLower(parsed.Hostname())
	if !contains(c.NetworkAllowlist, host) {
		return fmt.Errorf("%w: endpoint host is outside network allowlist", ErrInvalidConfig)
	}
	if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast()) && !cfg.AllowLoopback {
		return fmt.Errorf("%w: private endpoint requires an explicit local-server exception", ErrInvalidConfig)
	}
	if (host == "localhost" || strings.HasSuffix(host, ".localhost")) && !cfg.AllowLoopback {
		return fmt.Errorf("%w: loopback endpoint denied", ErrInvalidConfig)
	}
	return nil
}

func validateNetworks(networks []string) error {
	seen := map[string]struct{}{}
	for _, host := range networks {
		if host == "" || strings.ToLower(host) != host || strings.ContainsAny(host, "/*:@") || strings.HasSuffix(host, ".") {
			return fmt.Errorf("%w: network policy uses exact lowercase hosts", ErrInvalidConfig)
		}
		if _, exists := seen[host]; exists {
			return fmt.Errorf("%w: duplicate network host", ErrInvalidConfig)
		}
		seen[host] = struct{}{}
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err = io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func canonicalDirectory(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", errors.New("not a directory")
	}
	return filepath.Clean(resolved), nil
}

func within(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
