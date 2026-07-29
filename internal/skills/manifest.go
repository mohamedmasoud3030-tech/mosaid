package skills

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/policy"
)

const CurrentCoreVersion = "0.1.0"

var (
	ErrInvalidManifest  = errors.New("invalid skill manifest")
	ErrIntegrity        = errors.New("skill integrity verification failed")
	ErrConflict         = errors.New("skill version already registered")
	ErrIncompatible     = errors.New("skill is incompatible with this core")
	ErrInputSchema      = errors.New("skill input failed schema validation")
	manifestIDPattern   = regexp.MustCompile(`^[a-z][a-z0-9.-]{1,127}$`)
	propertyNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,127}$`)
	secretNamePattern   = regexp.MustCompile(`^[A-Z][A-Z0-9_]{1,127}$`)
	semanticVersionExpr = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[0-9A-Za-z.-]+)?$`)
)

type Type string

const (
	Declarative Type = "declarative"
	Builtin     Type = "builtin"
	MCP         Type = "mcp"
)

type ApprovalPolicy string

const (
	ApprovalByPolicy ApprovalPolicy = "policy"
	ApprovalAlways   ApprovalPolicy = "always"
	ApprovalBound    ApprovalPolicy = "bound_publish"
)

type Compatibility struct {
	MinCore string   `json:"min_core"`
	MaxCore string   `json:"max_core,omitempty"`
	OS      []string `json:"os,omitempty"`
	Arch    []string `json:"arch,omitempty"`
}

type Output struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`
}

type Step struct {
	Tool      string          `json:"tool"`
	Mode      policy.Mode     `json:"mode"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type MCPBinding struct {
	ServerID string `json:"server_id"`
	Tool     string `json:"tool"`
}

type Manifest struct {
	ID                  string          `json:"id"`
	Version             string          `json:"version"`
	Description         string          `json:"description"`
	Type                Type            `json:"type"`
	InputSchema         json.RawMessage `json:"input_schema"`
	Outputs             []Output        `json:"outputs"`
	RequiredTools       []string        `json:"required_tools"`
	RequiredPermissions []policy.Mode   `json:"required_permissions"`
	RequiredSecrets     []string        `json:"required_secrets"`
	AllowedNetworks     []string        `json:"allowed_networks"`
	TimeoutSeconds      int             `json:"timeout_seconds"`
	ApprovalPolicy      ApprovalPolicy  `json:"approval_policy"`
	Compatibility       Compatibility   `json:"compatibility"`
	IntegrityHash       string          `json:"integrity_hash"`
	Steps               []Step          `json:"steps,omitempty"`
	MCP                 *MCPBinding     `json:"mcp,omitempty"`
}

type Capabilities struct {
	Tools    map[string]struct{}
	Core     string
	OS       string
	Arch     string
	Networks map[string]struct{}
}

func (m Manifest) Validate(capabilities Capabilities) error {
	if !manifestIDPattern.MatchString(m.ID) {
		return fmt.Errorf("%w: invalid id", ErrInvalidManifest)
	}
	if !semanticVersionExpr.MatchString(m.Version) {
		return fmt.Errorf("%w: invalid semantic version", ErrInvalidManifest)
	}
	if len(strings.TrimSpace(m.Description)) < 8 || len(m.Description) > 1024 {
		return fmt.Errorf("%w: description length", ErrInvalidManifest)
	}
	if m.Type != Declarative && m.Type != Builtin && m.Type != MCP {
		return fmt.Errorf("%w: unsupported type", ErrInvalidManifest)
	}
	if err := validateSchemaDocument(m.InputSchema); err != nil {
		return fmt.Errorf("%w: input schema: %v", ErrInvalidManifest, err)
	}
	if len(m.Outputs) == 0 || len(m.Outputs) > 16 {
		return fmt.Errorf("%w: outputs required and bounded", ErrInvalidManifest)
	}
	seenOutputs := map[string]struct{}{}
	for _, output := range m.Outputs {
		if !manifestIDPattern.MatchString(output.Name) || strings.TrimSpace(output.Description) == "" {
			return fmt.Errorf("%w: invalid output", ErrInvalidManifest)
		}
		if _, exists := seenOutputs[output.Name]; exists {
			return fmt.Errorf("%w: duplicate output", ErrInvalidManifest)
		}
		seenOutputs[output.Name] = struct{}{}
		if err := validateSchemaDocument(output.Schema); err != nil {
			return fmt.Errorf("%w: output schema: %v", ErrInvalidManifest, err)
		}
	}
	if m.TimeoutSeconds < 1 || m.TimeoutSeconds > 3600 {
		return fmt.Errorf("%w: timeout out of bounds", ErrInvalidManifest)
	}
	if m.ApprovalPolicy != ApprovalByPolicy && m.ApprovalPolicy != ApprovalAlways && m.ApprovalPolicy != ApprovalBound {
		return fmt.Errorf("%w: invalid approval policy", ErrInvalidManifest)
	}
	if err := validateUniqueIdentifiers("tool", m.RequiredTools); err != nil {
		return err
	}
	for _, tool := range m.RequiredTools {
		if len(capabilities.Tools) != 0 {
			if _, ok := capabilities.Tools[tool]; !ok {
				return fmt.Errorf("%w: tool %q is not provided by the core", ErrInvalidManifest, tool)
			}
		}
	}
	permissions := map[policy.Mode]struct{}{}
	for _, permission := range m.RequiredPermissions {
		if permission != policy.Read && permission != policy.Write && permission != policy.Publish {
			return fmt.Errorf("%w: permission %q is unavailable to skills", ErrInvalidManifest, permission)
		}
		if _, exists := permissions[permission]; exists {
			return fmt.Errorf("%w: duplicate permission", ErrInvalidManifest)
		}
		permissions[permission] = struct{}{}
	}
	if len(permissions) == 0 {
		return fmt.Errorf("%w: required permissions missing", ErrInvalidManifest)
	}
	if _, publish := permissions[policy.Publish]; publish && m.ApprovalPolicy != ApprovalBound {
		return fmt.Errorf("%w: publishing requires bound approval", ErrInvalidManifest)
	}
	seenSecrets := map[string]struct{}{}
	for _, name := range m.RequiredSecrets {
		if !secretNamePattern.MatchString(name) {
			return fmt.Errorf("%w: invalid secret reference", ErrInvalidManifest)
		}
		if _, exists := seenSecrets[name]; exists {
			return fmt.Errorf("%w: duplicate secret reference", ErrInvalidManifest)
		}
		seenSecrets[name] = struct{}{}
	}
	if err := validateNetworks(m.AllowedNetworks, capabilities.Networks); err != nil {
		return err
	}
	if m.Compatibility.MinCore == "" || !semanticVersionExpr.MatchString(m.Compatibility.MinCore) {
		return fmt.Errorf("%w: min_core is required", ErrInvalidManifest)
	}
	if m.Compatibility.MaxCore != "" && !semanticVersionExpr.MatchString(m.Compatibility.MaxCore) {
		return fmt.Errorf("%w: invalid max_core", ErrInvalidManifest)
	}
	core := capabilities.Core
	if core == "" {
		core = CurrentCoreVersion
	}
	if compareVersions(core, m.Compatibility.MinCore) < 0 || (m.Compatibility.MaxCore != "" && compareVersions(core, m.Compatibility.MaxCore) > 0) {
		return ErrIncompatible
	}
	if capabilities.OS != "" && len(m.Compatibility.OS) != 0 && !contains(m.Compatibility.OS, capabilities.OS) {
		return ErrIncompatible
	}
	if capabilities.Arch != "" && len(m.Compatibility.Arch) != 0 && !contains(m.Compatibility.Arch, capabilities.Arch) {
		return ErrIncompatible
	}
	if err := m.validateImplementation(); err != nil {
		return err
	}
	if !validIntegrityShape(m.IntegrityHash) {
		return fmt.Errorf("%w: malformed integrity hash", ErrInvalidManifest)
	}
	return nil
}

func (m Manifest) validateImplementation() error {
	required := make(map[string]struct{}, len(m.RequiredTools))
	for _, tool := range m.RequiredTools {
		required[tool] = struct{}{}
	}
	switch m.Type {
	case Declarative:
		if m.MCP != nil || len(m.Steps) == 0 || len(m.Steps) > 32 {
			return fmt.Errorf("%w: declarative steps required and bounded", ErrInvalidManifest)
		}
		for _, step := range m.Steps {
			if _, ok := required[step.Tool]; !ok {
				return fmt.Errorf("%w: step tool is not declared", ErrInvalidManifest)
			}
			if step.Mode != policy.Read && step.Mode != policy.Write && step.Mode != policy.Publish {
				return fmt.Errorf("%w: invalid step mode", ErrInvalidManifest)
			}
			if !containsMode(m.RequiredPermissions, step.Mode) {
				return fmt.Errorf("%w: step widens permission scope", ErrInvalidManifest)
			}
			if len(step.Arguments) != 0 && !json.Valid(step.Arguments) {
				return fmt.Errorf("%w: malformed step arguments", ErrInvalidManifest)
			}
		}
	case Builtin:
		if m.MCP != nil || len(m.Steps) != 0 {
			return fmt.Errorf("%w: builtin implementation must be compiled in", ErrInvalidManifest)
		}
	case MCP:
		if len(m.Steps) != 0 || m.MCP == nil || !manifestIDPattern.MatchString(m.MCP.ServerID) || !manifestIDPattern.MatchString(m.MCP.Tool) {
			return fmt.Errorf("%w: pinned MCP binding required", ErrInvalidManifest)
		}
	}
	return nil
}

func (m Manifest) VerifyIntegrity() error {
	want := strings.TrimPrefix(m.IntegrityHash, "sha256:")
	got, err := m.ComputeIntegrity()
	if err != nil {
		return err
	}
	if want == "" || !strings.EqualFold(want, strings.TrimPrefix(got, "sha256:")) {
		return ErrIntegrity
	}
	return nil
}

func (m Manifest) ComputeIntegrity() (string, error) {
	m.IntegrityHash = ""
	encoded, err := json.Marshal(m)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(hash[:]), nil
}

func validIntegrityShape(value string) bool {
	if !strings.HasPrefix(value, "sha256:") || len(value) != len("sha256:")+64 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, "sha256:"))
	return err == nil
}

func validateUniqueIdentifiers(kind string, values []string) error {
	seen := map[string]struct{}{}
	for _, value := range values {
		if !manifestIDPattern.MatchString(value) {
			return fmt.Errorf("%w: invalid %s identifier", ErrInvalidManifest, kind)
		}
		if _, exists := seen[value]; exists {
			return fmt.Errorf("%w: duplicate %s", ErrInvalidManifest, kind)
		}
		seen[value] = struct{}{}
	}
	return nil
}

func validateNetworks(networks []string, available map[string]struct{}) error {
	seen := map[string]struct{}{}
	for _, network := range networks {
		host := strings.ToLower(strings.TrimSuffix(network, "."))
		if host == "" || host != network || strings.ContainsAny(host, "/*:@") || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
			return fmt.Errorf("%w: network scope must be an exact DNS name", ErrInvalidManifest)
		}
		if ip := net.ParseIP(host); ip != nil || host == "localhost" || strings.HasSuffix(host, ".localhost") {
			return fmt.Errorf("%w: IP and local network scopes are forbidden", ErrInvalidManifest)
		}
		if _, exists := seen[host]; exists {
			return fmt.Errorf("%w: duplicate network scope", ErrInvalidManifest)
		}
		seen[host] = struct{}{}
		if len(available) != 0 {
			if _, ok := available[host]; !ok {
				return fmt.Errorf("%w: network scope %q is not granted by the core", ErrInvalidManifest, host)
			}
		}
	}
	return nil
}

func validateSchemaDocument(raw json.RawMessage) error {
	if len(raw) == 0 || len(raw) > 64*1024 || !json.Valid(raw) {
		return errors.New("schema must be valid and bounded JSON")
	}
	var schema any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&schema); err != nil {
		return err
	}
	return validateSchemaNode(schema, 0)
}

func validateSchemaNode(node any, depth int) error {
	if depth > 16 {
		return errors.New("schema nesting limit exceeded")
	}
	object, ok := node.(map[string]any)
	if !ok {
		return errors.New("schema node must be an object")
	}
	allowed := map[string]struct{}{"type": {}, "description": {}, "properties": {}, "required": {}, "additionalProperties": {}, "items": {}, "enum": {}, "minLength": {}, "maxLength": {}, "minimum": {}, "maximum": {}, "minItems": {}, "maxItems": {}}
	for key := range object {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("unsupported schema keyword %q", key)
		}
	}
	typeName, ok := object["type"].(string)
	if !ok || !contains([]string{"object", "string", "number", "integer", "boolean", "array", "null"}, typeName) {
		return errors.New("explicit supported schema type required")
	}
	if typeName == "object" {
		properties, ok := object["properties"].(map[string]any)
		if !ok {
			return errors.New("object properties required")
		}
		if additional, exists := object["additionalProperties"]; !exists || additional != false {
			return errors.New("additionalProperties must be false")
		}
		for name, child := range properties {
			if !propertyNamePattern.MatchString(name) {
				return errors.New("invalid property name")
			}
			if err := validateSchemaNode(child, depth+1); err != nil {
				return err
			}
		}
		if required, exists := object["required"]; exists {
			values, ok := required.([]any)
			if !ok {
				return errors.New("required must be an array")
			}
			for _, item := range values {
				name, ok := item.(string)
				if !ok {
					return errors.New("required property must be a string")
				}
				if _, exists := properties[name]; !exists {
					return errors.New("required property is not declared")
				}
			}
		}
	}
	if typeName == "array" {
		if err := validateSchemaNode(object["items"], depth+1); err != nil {
			return fmt.Errorf("array items: %w", err)
		}
	}
	return nil
}

func compareVersions(left, right string) int {
	parse := func(value string) [3]int {
		value = strings.SplitN(value, "-", 2)[0]
		parts := strings.Split(value, ".")
		var result [3]int
		for i := 0; i < len(parts) && i < 3; i++ {
			result[i], _ = strconv.Atoi(parts[i])
		}
		return result
	}
	a, b := parse(left), parse(right)
	for i := range a {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

func contains(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func containsMode(values []policy.Mode, wanted policy.Mode) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func sortedVersions(manifests map[string]Manifest) []string {
	versions := make([]string, 0, len(manifests))
	for version := range manifests {
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool { return compareVersions(versions[i], versions[j]) > 0 })
	return versions
}
