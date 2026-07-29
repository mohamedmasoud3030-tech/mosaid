// Package benchmark implements Mosaid's benchmark harness for testing
// the cognitive engine and provider platform with mock-based scenarios.
//
// All benchmarks use mock providers — no real API calls are made.
package benchmark

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// Scenario represents a benchmark test scenario.
type Scenario struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	Category    string        `json:"category"` // "arabic", "coding", "provider", "security", "general"
	Timeout     time.Duration `json:"timeout"`
	Execute     func(ctx context.Context, env *Environment) (Result, error)
}

// Environment provides the benchmark environment.
type Environment struct {
	Providers map[string]MockProviderConfig
	Storage   BenchmarkStore
}

// MockProviderConfig configures a mock provider for benchmarks.
type MockProviderConfig struct {
	ID       string
	Scenario string // "success", "failure", "slow", etc.
	Responses []string // Pre-defined responses
}

// BenchmarkStore provides storage for benchmark runs.
type BenchmarkStore interface {
	SaveResult(ctx context.Context, result Result) error
	ListResults(ctx context.Context, suite string) ([]Result, error)
}

// Result records the outcome of a benchmark scenario.
type Result struct {
	ScenarioID  string        `json:"scenario_id"`
	Suite       string        `json:"suite"`
	Passed      bool          `json:"passed"`
	Duration    time.Duration `json:"duration"`
	Error       string        `json:"error,omitempty"`
	Details     string        `json:"details,omitempty"`
	ProviderUsed string       `json:"provider_used,omitempty"`
	TokensUsed  int           `json:"tokens_used,omitempty"`
	Timestamp   time.Time     `json:"timestamp"`
}

// Suite represents a collection of benchmark scenarios.
type Suite struct {
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Scenarios   []Scenario `json:"-"`
}

// CoreSuite returns the core benchmark scenarios.
func CoreSuite() *Suite {
	s := &Suite{
		Name:        "core",
		Description: "Core cognitive engine benchmarks",
	}
	s.Scenarios = []Scenario{
		{
			ID:          "core-01",
			Name:        "understand_egyptian_dialect",
			Description: "Understand a request in Egyptian Arabic dialect",
			Category:    "arabic",
			Timeout:     30 * time.Second,
			Execute:     benchEgyptianDialect,
		},
		{
			ID:          "core-02",
			Name:        "ambiguous_to_structured",
			Description: "Convert vague request to structured goal + success criteria",
			Category:    "general",
			Timeout:     30 * time.Second,
			Execute:     benchAmbiguousToStructured,
		},
		{
			ID:          "core-03",
			Name:        "multi_step_plan",
			Description: "Create a multi-step plan from a complex request",
			Category:    "general",
			Timeout:     30 * time.Second,
			Execute:     benchMultiStepPlan,
		},
		{
			ID:          "core-04",
			Name:        "json_schema_output",
			Description: "Produce JSON output matching a specific schema",
			Category:    "general",
			Timeout:     30 * time.Second,
			Execute:     benchJSONSchemaOutput,
		},
		{
			ID:          "core-05",
			Name:        "tool_selection",
			Description: "Select the correct tool for a given task",
			Category:    "general",
			Timeout:     30 * time.Second,
			Execute:     benchToolSelection,
		},
		{
			ID:          "core-06",
			Name:        "tool_permission_refusal",
			Description: "Refuse to use a tool not in allowed list",
			Category:    "security",
			Timeout:     30 * time.Second,
			Execute:     benchToolPermissionRefusal,
		},
		{
			ID:          "core-07",
			Name:        "go_code_generation",
			Description: "Generate valid Go code for a simple function",
			Category:    "coding",
			Timeout:     30 * time.Second,
			Execute:     benchGoCodeGeneration,
		},
		{
			ID:          "core-08",
			Name:        "bug_detection",
			Description: "Detect a bug in provided code",
			Category:    "coding",
			Timeout:     30 * time.Second,
			Execute:     benchBugDetection,
		},
		{
			ID:          "core-09",
			Name:        "diff_review",
			Description: "Review a code diff for issues",
			Category:    "coding",
			Timeout:     30 * time.Second,
			Execute:     benchDiffReview,
		},
		{
			ID:          "core-10",
			Name:        "arabic_document_summary",
			Description: "Summarize an Arabic document",
			Category:    "arabic",
			Timeout:     30 * time.Second,
			Execute:     benchArabicSummary,
		},
		{
			ID:          "core-11",
			Name:        "prompt_injection_resistance",
			Description: "Resist prompt injection attempts",
			Category:    "security",
			Timeout:     30 * time.Second,
			Execute:     benchPromptInjection,
		},
		{
			ID:          "core-12",
			Name:        "no_hallucination",
			Description: "Refuse to fabricate information when unsure",
			Category:    "security",
			Timeout:     30 * time.Second,
			Execute:     benchNoHallucination,
		},
		{
			ID:          "core-13",
			Name:        "stop_without_evidence",
			Description: "Stop execution when evidence is missing",
			Category:    "security",
			Timeout:     30 * time.Second,
			Execute:     benchStopWithoutEvidence,
		},
		{
			ID:          "core-14",
			Name:        "free_only_enforcement",
			Description: "Enforce free_only policy under all conditions",
			Category:    "provider",
			Timeout:     30 * time.Second,
			Execute:     benchFreeOnlyEnforcement,
		},
	}
	return s
}

// ProviderSuite returns provider-specific benchmark scenarios.
func ProviderSuite() *Suite {
	s := &Suite{
		Name:        "provider",
		Description: "Provider platform benchmarks (mock-based)",
	}
	s.Scenarios = []Scenario{
		{
			ID:          "prov-01",
			Name:        "kaggle_session_expiry_fallback",
			Description: "Kaggle session expires → fallback to Groq",
			Category:    "provider",
			Timeout:     30 * time.Second,
			Execute:     benchKaggleSessionExpiry,
		},
		{
			ID:          "prov-02",
			Name:        "g4f_endpoint_rotation",
			Description: "G4F endpoint changes → fallback to next provider",
			Category:    "provider",
			Timeout:     30 * time.Second,
			Execute:     benchG4FEndpointRotation,
		},
		{
			ID:          "prov-03",
			Name:        "mlc_slow_response_timeout",
			Description: "MLC slow response → timeout → fallback",
			Category:    "provider",
			Timeout:     30 * time.Second,
			Execute:     benchMLCSlowTimeout,
		},
		{
			ID:          "prov-04",
			Name:        "all_free_failed_persist_and_stop",
			Description: "All free providers fail → persist state and stop (no paid attempt)",
			Category:    "provider",
			Timeout:     30 * time.Second,
			Execute:     benchAllFreeFailedStop,
		},
	}
	return s
}

// AllSuites returns all benchmark suites.
func AllSuites() []*Suite {
	return []*Suite{CoreSuite(), ProviderSuite()}
}

// RunSuite executes all scenarios in a suite.
func RunSuite(ctx context.Context, suite *Suite, env *Environment) []Result {
	results := make([]Result, 0, len(suite.Scenarios))
	for _, scenario := range suite.Scenarios {
		result := RunScenario(ctx, scenario, env, suite.Name)
		results = append(results, result)
	}
	return results
}

// RunScenario executes a single benchmark scenario.
func RunScenario(ctx context.Context, scenario Scenario, env *Environment, suiteName string) Result {
	start := time.Now()

	scenarioCtx, cancel := context.WithTimeout(ctx, scenario.Timeout)
	defer cancel()

	result := Result{
		ScenarioID: scenario.ID,
		Suite:      suiteName,
		Timestamp:  start,
	}

	scenarioResult, err := scenario.Execute(scenarioCtx, env)
	result.Duration = time.Since(start)

	if err != nil {
		result.Passed = false
		result.Error = err.Error()
	} else {
		result.Passed = scenarioResult.Passed
		result.Details = scenarioResult.Details
		result.ProviderUsed = scenarioResult.ProviderUsed
		result.TokensUsed = scenarioResult.TokensUsed
		if scenarioResult.Error != "" {
			result.Error = scenarioResult.Error
		}
	}

	return result
}

// Summary returns a human-readable summary of benchmark results.
func Summary(results []Result) string {
	passed, failed := 0, 0
	for _, r := range results {
		if r.Passed {
			passed++
		} else {
			failed++
		}
	}

	b, _ := json.MarshalIndent(struct {
		Total   int `json:"total"`
		Passed  int `json:"passed"`
		Failed  int `json:"failed"`
		Results []Result `json:"results"`
	}{len(results), passed, failed, results}, "", "  ")

	return fmt.Sprintf("Benchmark: %d/%d passed\n%s", passed, len(results), string(b))
}
