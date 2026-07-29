package providers_test

import (
	"testing"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/model/providers"
	"github.com/mohamedmasoud3030-tech/mosaid/internal/model/providers/mock"
)

func TestRegistryRegisterAndList(t *testing.T) {
	reg, err := providers.NewRegistry(providers.DefaultFreeOnlyPolicy())
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}

	p1 := mock.New(mock.Config{ID: "p1", Tier: providers.TierVerifiedFree})
	p2 := mock.New(mock.Config{ID: "p2", Tier: providers.TierLocalSelfHosted})

	if err := reg.Register(p1); err != nil {
		t.Fatalf("Register p1: %v", err)
	}
	if err := reg.Register(p2); err != nil {
		t.Fatalf("Register p2: %v", err)
	}

	if reg.Count() != 2 {
		t.Errorf("Count() = %d, want 2", reg.Count())
	}

	list := reg.List()
	if len(list) != 2 {
		t.Errorf("List() returned %d providers, want 2", len(list))
	}
}

func TestRegistryDuplicateRegistration(t *testing.T) {
	reg, _ := providers.NewRegistry(providers.DefaultFreeOnlyPolicy())

	p := mock.New(mock.Config{ID: "dup", Tier: providers.TierVerifiedFree})
	if err := reg.Register(p); err != nil {
		t.Fatalf("first Register: %v", err)
	}
	if err := reg.Register(p); err == nil {
		t.Error("duplicate registration should fail")
	}
}

func TestRegistryRejectsPaidTier(t *testing.T) {
	reg, _ := providers.NewRegistry(providers.DefaultFreeOnlyPolicy())

	paid := mock.New(mock.Config{ID: "paid", Tier: providers.TierPaidOnly})
	if err := reg.Register(paid); err == nil {
		t.Error("paid tier should be rejected in free_only mode")
	}

	unknown := mock.New(mock.Config{ID: "unknown", Tier: providers.TierUnknown})
	if err := reg.Register(unknown); err == nil {
		t.Error("unknown tier should be rejected in free_only mode")
	}

	card := mock.New(mock.Config{ID: "card", Tier: providers.TierFreeWithCard})
	if err := reg.Register(card); err == nil {
		t.Error("free_with_card tier should be rejected in free_only mode")
	}
}

func TestRegistryGet(t *testing.T) {
	reg, _ := providers.NewRegistry(providers.DefaultFreeOnlyPolicy())

	p := mock.New(mock.Config{ID: "test", Tier: providers.TierVerifiedFree})
	reg.Register(p)

	got, err := reg.Get("test")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID() != "test" {
		t.Errorf("Get().ID() = %q, want %q", got.ID(), "test")
	}

	_, err = reg.Get("nonexistent")
	if err == nil {
		t.Error("Get nonexistent should fail")
	}
}

func TestRegistryListByTier(t *testing.T) {
	reg, _ := providers.NewRegistry(providers.DefaultFreeOnlyPolicy())

	p1 := mock.New(mock.Config{ID: "free1", Tier: providers.TierVerifiedFree})
	p2 := mock.New(mock.Config{ID: "local1", Tier: providers.TierLocalSelfHosted})
	p3 := mock.New(mock.Config{ID: "free2", Tier: providers.TierVerifiedFree})

	reg.Register(p1)
	reg.Register(p2)
	reg.Register(p3)

	free := reg.ListByTier(providers.TierVerifiedFree)
	if len(free) != 2 {
		t.Errorf("ListByTier(verified_free) = %d, want 2", len(free))
	}

	local := reg.ListByTier(providers.TierLocalSelfHosted)
	if len(local) != 1 {
		t.Errorf("ListByTier(local_self_hosted) = %d, want 1", len(local))
	}
}

func TestKaggleIsLocalSelfHosted(t *testing.T) {
	// Verify Kaggle is classified as local_self_hosted
	reg, _ := providers.NewRegistry(providers.DefaultFreeOnlyPolicy())
	p := mock.New(mock.Config{ID: "kaggle", Tier: providers.TierLocalSelfHosted})
	if err := reg.Register(p); err != nil {
		t.Fatalf("kaggle (local_self_hosted) should be allowed in free_only: %v", err)
	}
}

func TestG4FIsLocalSelfHosted(t *testing.T) {
	// Verify G4F is classified as local_self_hosted
	reg, _ := providers.NewRegistry(providers.DefaultFreeOnlyPolicy())
	p := mock.New(mock.Config{ID: "g4f", Tier: providers.TierLocalSelfHosted})
	if err := reg.Register(p); err != nil {
		t.Fatalf("g4f (local_self_hosted) should be allowed in free_only: %v", err)
	}
}

func TestRegistryPolicy(t *testing.T) {
	policy := providers.DefaultFreeOnlyPolicy()
	reg, _ := providers.NewRegistry(policy)

	got := reg.Policy()
	if got.Mode != providers.BillingModeFreeOnly {
		t.Errorf("Policy().Mode = %q, want free_only", got.Mode)
	}
}

func TestNilProviderRegistration(t *testing.T) {
	reg, _ := providers.NewRegistry(providers.DefaultFreeOnlyPolicy())
	if err := reg.Register(nil); err == nil {
		t.Error("nil provider should be rejected")
	}
}

func TestEmptyIDRegistration(t *testing.T) {
	reg, _ := providers.NewRegistry(providers.DefaultFreeOnlyPolicy())
	p := mock.New(mock.Config{ID: "", Tier: providers.TierVerifiedFree})
	if err := reg.Register(p); err == nil {
		t.Error("empty ID should be rejected")
	}
}
