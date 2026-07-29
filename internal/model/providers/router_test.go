package providers_test

import (
	"context"
	"testing"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/model/providers"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/model/providers/mock"
)

func TestRouterRouteSuccess(t *testing.T) {
	reg, _ := providers.NewRegistry(providers.DefaultFreeOnlyPolicy())
	p := mock.New(mock.Config{ID: "primary", Tier: providers.TierVerifiedFree, Scenario: mock.ScenarioSuccess})
	reg.Register(p)

	router := providers.NewRouter(reg)
	router.SetChain(providers.TaskGeneral, []providers.FallbackEntry{
		{ProviderID: "primary", Tier: providers.TierVerifiedFree, When: "always"},
	})

	result, err := router.Route(context.Background(), providers.TaskGeneral, providers.Capabilities{Text: true})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if result.Provider.ID() != "primary" {
		t.Errorf("Route().Provider.ID() = %q, want %q", result.Provider.ID(), "primary")
	}
}

func TestRouterRouteFallback(t *testing.T) {
	reg, _ := providers.NewRegistry(providers.DefaultFreeOnlyPolicy())
	primary := mock.New(mock.Config{ID: "primary", Tier: providers.TierVerifiedFree, Scenario: mock.ScenarioPermanentFailure})
	backup := mock.New(mock.Config{ID: "backup", Tier: providers.TierVerifiedFree, Scenario: mock.ScenarioSuccess})
	reg.Register(primary)
	reg.Register(backup)

	router := providers.NewRouter(reg)
	router.SetChain(providers.TaskGeneral, []providers.FallbackEntry{
		{ProviderID: "primary", Tier: providers.TierVerifiedFree, When: "always"},
		{ProviderID: "backup", Tier: providers.TierVerifiedFree, When: "always"},
	})

	result, err := router.Route(context.Background(), providers.TaskGeneral, providers.Capabilities{Text: true})
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if result.Provider.ID() != "backup" {
		t.Errorf("Route().Provider.ID() = %q, want backup", result.Provider.ID())
	}
	if len(result.Tried) != 1 || result.Tried[0] != "primary" {
		t.Errorf("Tried = %v, want [primary]", result.Tried)
	}
}

func TestRouterRouteAllExhausted(t *testing.T) {
	reg, _ := providers.NewRegistry(providers.DefaultFreeOnlyPolicy())
	p1 := mock.New(mock.Config{ID: "p1", Tier: providers.TierVerifiedFree, Scenario: mock.ScenarioPermanentFailure})
	p2 := mock.New(mock.Config{ID: "p2", Tier: providers.TierVerifiedFree, Scenario: mock.ScenarioPermanentFailure})
	reg.Register(p1)
	reg.Register(p2)

	router := providers.NewRouter(reg)
	router.SetChain(providers.TaskGeneral, []providers.FallbackEntry{
		{ProviderID: "p1", Tier: providers.TierVerifiedFree, When: "always"},
		{ProviderID: "p2", Tier: providers.TierVerifiedFree, When: "always"},
		{Action: "persist_and_stop"},
	})

	_, err := router.Route(context.Background(), providers.TaskGeneral, providers.Capabilities{Text: true})
	if err == nil {
		t.Error("should fail when all providers exhausted")
	}
}

func TestRouterNoChainConfigured(t *testing.T) {
	reg, _ := providers.NewRegistry(providers.DefaultFreeOnlyPolicy())
	router := providers.NewRouter(reg)

	_, err := router.Route(context.Background(), providers.TaskGeneral, providers.Capabilities{Text: true})
	if err == nil {
		t.Error("should fail when no chain configured")
	}
}

func TestRouterGenerateSuccess(t *testing.T) {
	reg, _ := providers.NewRegistry(providers.DefaultFreeOnlyPolicy())
	p := mock.New(mock.Config{ID: "gen", Tier: providers.TierVerifiedFree, Scenario: mock.ScenarioSuccess})
	reg.Register(p)

	router := providers.NewRouter(reg)
	router.SetChain(providers.TaskGeneral, []providers.FallbackEntry{
		{ProviderID: "gen", Tier: providers.TierVerifiedFree, When: "always"},
	})

	resp, err := router.Generate(context.Background(), providers.TaskGeneral, providers.Request{
		Messages: []providers.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Content != "mock response" {
		t.Errorf("Generate().Content = %q, want %q", resp.Content, "mock response")
	}
}

func TestRouterGenerateRejectsNonzeroCost(t *testing.T) {
	reg, _ := providers.NewRegistry(providers.DefaultFreeOnlyPolicy())
	p := mock.New(mock.Config{ID: "costly", Tier: providers.TierVerifiedFree, Scenario: mock.ScenarioNonzeroCostRejection})
	reg.Register(p)

	router := providers.NewRouter(reg)
	router.SetChain(providers.TaskGeneral, []providers.FallbackEntry{
		{ProviderID: "costly", Tier: providers.TierVerifiedFree, When: "always"},
		{Action: "persist_and_stop"},
	})

	_, err := router.Generate(context.Background(), providers.TaskGeneral, providers.Request{
		Messages: []providers.Message{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Error("should reject provider with nonzero cost")
	}
}

func TestRouterGenerateFallbackAfterFailure(t *testing.T) {
	reg, _ := providers.NewRegistry(providers.DefaultFreeOnlyPolicy())
	fail := mock.New(mock.Config{ID: "fail", Tier: providers.TierVerifiedFree, Scenario: mock.ScenarioTemporaryFailure})
	ok := mock.New(mock.Config{ID: "ok", Tier: providers.TierVerifiedFree, Scenario: mock.ScenarioSuccess})
	reg.Register(fail)
	reg.Register(ok)

	router := providers.NewRouter(reg)
	router.SetChain(providers.TaskGeneral, []providers.FallbackEntry{
		{ProviderID: "fail", Tier: providers.TierVerifiedFree, When: "always"},
		{ProviderID: "ok", Tier: providers.TierVerifiedFree, When: "always"},
	})

	resp, err := router.Generate(context.Background(), providers.TaskGeneral, providers.Request{
		Messages: []providers.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Generate with fallback: %v", err)
	}
	if resp.Provider != "ok" {
		t.Errorf("Provider = %q, want ok", resp.Provider)
	}
}

func TestRouterHealthSnapshot(t *testing.T) {
	reg, _ := providers.NewRegistry(providers.DefaultFreeOnlyPolicy())
	p := mock.New(mock.Config{ID: "hs", Tier: providers.TierVerifiedFree, Scenario: mock.ScenarioSuccess})
	reg.Register(p)

	router := providers.NewRouter(reg)
	router.SetChain(providers.TaskGeneral, []providers.FallbackEntry{
		{ProviderID: "hs", Tier: providers.TierVerifiedFree, When: "always"},
	})

	// Trigger a route to populate health cache
	router.Route(context.Background(), providers.TaskGeneral, providers.Capabilities{Text: true})

	snapshot := router.HealthSnapshot()
	if len(snapshot) == 0 {
		t.Error("HealthSnapshot should have entries after routing")
	}
}

func TestRouterAllStatus(t *testing.T) {
	reg, _ := providers.NewRegistry(providers.DefaultFreeOnlyPolicy())
	p1 := mock.New(mock.Config{ID: "s1", Tier: providers.TierVerifiedFree})
	p2 := mock.New(mock.Config{ID: "s2", Tier: providers.TierLocalSelfHosted})
	reg.Register(p1)
	reg.Register(p2)

	router := providers.NewRouter(reg)
	statuses := router.AllStatus(context.Background())
	if len(statuses) != 2 {
		t.Errorf("AllStatus() = %d entries, want 2", len(statuses))
	}
}

func TestRouterKaggleSessionExpiredFallsBack(t *testing.T) {
	reg, _ := providers.NewRegistry(providers.DefaultFreeOnlyPolicy())
	kaggle := mock.New(mock.Config{ID: "kaggle", Tier: providers.TierLocalSelfHosted, Scenario: mock.ScenarioKaggleSessionExpired})
	groq := mock.New(mock.Config{ID: "groq", Tier: providers.TierVerifiedFree, Scenario: mock.ScenarioSuccess})
	reg.Register(kaggle)
	reg.Register(groq)

	router := providers.NewRouter(reg)
	router.SetChain(providers.TaskPlanning, []providers.FallbackEntry{
		{ProviderID: "kaggle", Tier: providers.TierLocalSelfHosted, When: "session_active"},
		{ProviderID: "groq", Tier: providers.TierVerifiedFree, When: "always"},
	})

	result, err := router.Route(context.Background(), providers.TaskPlanning, providers.Capabilities{Text: true})
	if err != nil {
		t.Fatalf("Route with kaggle expired: %v", err)
	}
	if result.Provider.ID() != "groq" {
		t.Errorf("Route().Provider.ID() = %q, want groq", result.Provider.ID())
	}
}

func TestRouterG4FEndpointRotatedFallsBack(t *testing.T) {
	reg, _ := providers.NewRegistry(providers.DefaultFreeOnlyPolicy())
	g4f := mock.New(mock.Config{ID: "g4f", Tier: providers.TierLocalSelfHosted, Scenario: mock.ScenarioG4FEndpointRotated})
	mlc := mock.New(mock.Config{ID: "mlc", Tier: providers.TierLocalSelfHosted, Scenario: mock.ScenarioSuccess})
	reg.Register(g4f)
	reg.Register(mlc)

	router := providers.NewRouter(reg)
	router.SetChain(providers.TaskPlanning, []providers.FallbackEntry{
		{ProviderID: "g4f", Tier: providers.TierLocalSelfHosted, When: "emergency_only"},
		{ProviderID: "mlc", Tier: providers.TierLocalSelfHosted, When: "offline_or_all_failed"},
	})

	// Health check should show g4f as up (it's the scenario that fails in Generate, not Health)
	// So route will select g4f first, then Generate will fail and fallback to mlc
	resp, err := router.Generate(context.Background(), providers.TaskPlanning, providers.Request{
		Messages: []providers.Message{{Role: "user", Content: "hello"}},
	})
	if err != nil {
		t.Fatalf("Generate with g4f rotated: %v", err)
	}
	if resp.Provider != "mlc" {
		t.Errorf("Provider = %q, want mlc", resp.Provider)
	}
}

func TestRouterNoPaidAttemptedWhenAllFreeFail(t *testing.T) {
	reg, _ := providers.NewRegistry(providers.DefaultFreeOnlyPolicy())
	// Only register free providers that all fail
	p1 := mock.New(mock.Config{ID: "f1", Tier: providers.TierVerifiedFree, Scenario: mock.ScenarioPermanentFailure})
	p2 := mock.New(mock.Config{ID: "f2", Tier: providers.TierLocalSelfHosted, Scenario: mock.ScenarioPermanentFailure})
	reg.Register(p1)
	reg.Register(p2)

	router := providers.NewRouter(reg)
	router.SetChain(providers.TaskGeneral, []providers.FallbackEntry{
		{ProviderID: "f1", Tier: providers.TierVerifiedFree, When: "always"},
		{ProviderID: "f2", Tier: providers.TierLocalSelfHosted, When: "always"},
		{Action: "persist_and_stop"},
	})

	_, err := router.Generate(context.Background(), providers.TaskGeneral, providers.Request{
		Messages: []providers.Message{{Role: "user", Content: "hello"}},
	})
	if err == nil {
		t.Error("should fail when all free providers exhausted")
	}
	// Verify no paid provider was attempted (it wasn't registered)
}
