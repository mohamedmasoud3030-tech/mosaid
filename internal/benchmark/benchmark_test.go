package benchmark_test

import (
	"context"
	"testing"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/benchmark"
)

func TestCoreSuiteHas14Scenarios(t *testing.T) {
	suite := benchmark.CoreSuite()
	if len(suite.Scenarios) != 14 {
		t.Errorf("CoreSuite() = %d scenarios, want 14", len(suite.Scenarios))
	}
}

func TestProviderSuiteHas4Scenarios(t *testing.T) {
	suite := benchmark.ProviderSuite()
	if len(suite.Scenarios) != 4 {
		t.Errorf("ProviderSuite() = %d scenarios, want 4", len(suite.Scenarios))
	}
}

func TestAllSuites(t *testing.T) {
	suites := benchmark.AllSuites()
	if len(suites) != 2 {
		t.Errorf("AllSuites() = %d, want 2", len(suites))
	}

	total := 0
	for _, s := range suites {
		total += len(s.Scenarios)
	}
	if total != 18 {
		t.Errorf("Total scenarios = %d, want 18", total)
	}
}

func TestRunSuiteCore(t *testing.T) {
	env := &benchmark.Environment{}
	suite := benchmark.CoreSuite()

	results := benchmark.RunSuite(context.Background(), suite, env)

	if len(results) != 14 {
		t.Errorf("RunSuite returned %d results, want 14", len(results))
	}

	passed := 0
	for _, r := range results {
		if r.Passed {
			passed++
		}
	}
	if passed != 14 {
		t.Errorf("Core suite: %d/14 passed", passed)
	}
}

func TestRunSuiteProvider(t *testing.T) {
	env := &benchmark.Environment{}
	suite := benchmark.ProviderSuite()

	results := benchmark.RunSuite(context.Background(), suite, env)

	if len(results) != 4 {
		t.Errorf("RunSuite returned %d results, want 4", len(results))
	}

	passed := 0
	for _, r := range results {
		if r.Passed {
			passed++
		}
	}
	if passed != 4 {
		t.Errorf("Provider suite: %d/4 passed", passed)
	}
}

func TestRunSuiteAllPass(t *testing.T) {
	env := &benchmark.Environment{}
	suites := benchmark.AllSuites()

	for _, suite := range suites {
		results := benchmark.RunSuite(context.Background(), suite, env)
		for _, r := range results {
			if !r.Passed {
				t.Errorf("Scenario %s failed: %s", r.ScenarioID, r.Error)
			}
		}
	}
}

func TestBenchmarkSummary(t *testing.T) {
	results := []benchmark.Result{
		{ScenarioID: "1", Passed: true},
		{ScenarioID: "2", Passed: false, Error: "test error"},
		{ScenarioID: "3", Passed: true},
	}

	summary := benchmark.Summary(results)
	if summary == "" {
		t.Error("Summary should not be empty")
	}
	// Summary should contain "2/3 passed"
	if !contains(summary, "2/3") {
		t.Errorf("Summary should contain pass count: %s", summary)
	}
}

func TestScenarioTimeout(t *testing.T) {
	scenario := benchmark.Scenario{
		ID:      "timeout-test",
		Timeout: 1, // 1 nanosecond — will be expired
		Execute: func(ctx context.Context, env *benchmark.Environment) (benchmark.Result, error) {
			<-ctx.Done()
			return benchmark.Result{Passed: false}, ctx.Err()
		},
	}

	env := &benchmark.Environment{}
	result := benchmark.RunScenario(context.Background(), scenario, env, "test")
	// Should still complete (context will be cancelled)
	if result.Duration <= 0 {
		t.Error("Duration should be positive")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
