package cognitive_test

import (
	"testing"

	"github.com/mohamedmasoud3030-tech/mosaid/internal/cognitive"
)

func TestDefaultWorkPacksComplete(t *testing.T) {
	packs := cognitive.DefaultWorkPacks()
	if len(packs) != 6 {
		t.Fatalf("DefaultWorkPacks() = %d, want 6", len(packs))
	}

	for _, wp := range packs {
		t.Run(wp.ID, func(t *testing.T) {
			if err := wp.CompletenessCheck(); err != nil {
				t.Errorf("WorkPack %q completeness check failed: %v", wp.ID, err)
			}
			if wp.Status != cognitive.WorkPackComplete {
				t.Errorf("WorkPack %q status = %q, want complete", wp.ID, wp.Status)
			}
		})
	}
}

func TestWorkPackCompletenessChecks(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*cognitive.WorkPack)
		wantErr bool
	}{
		{"valid", func(wp *cognitive.WorkPack) {}, false},
		{"empty ID", func(wp *cognitive.WorkPack) { wp.ID = "" }, true},
		{"invalid ID", func(wp *cognitive.WorkPack) { wp.ID = "INVALID" }, true},
		{"empty name", func(wp *cognitive.WorkPack) { wp.Name = "" }, true},
		{"empty description", func(wp *cognitive.WorkPack) { wp.Description = "" }, true},
		{"empty version", func(wp *cognitive.WorkPack) { wp.Version = "" }, true},
		{"empty intake", func(wp *cognitive.WorkPack) { wp.IntakeSchema = nil }, true},
		{"empty workflow", func(wp *cognitive.WorkPack) { wp.Workflow = nil }, true},
		{"empty tools", func(wp *cognitive.WorkPack) { wp.RequiredTools = nil }, true},
		{"empty quality", func(wp *cognitive.WorkPack) { wp.QualityChecks = nil }, true},
		{"empty format", func(wp *cognitive.WorkPack) { wp.DeliveryFormat = "" }, true},
		{"short guide", func(wp *cognitive.WorkPack) { wp.BeginnerGuide = "short" }, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wp := validTestWorkPack()
			tt.modify(wp)
			err := wp.CompletenessCheck()
			if (err != nil) != tt.wantErr {
				t.Errorf("CompletenessCheck() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWorkPackRegistry(t *testing.T) {
	reg := cognitive.NewWorkPackRegistry()

	wp := validTestWorkPack()
	if err := reg.Register(wp); err != nil {
		t.Fatalf("Register: %v", err)
	}

	got, err := reg.Get("test-pack")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.ID != "test-pack" {
		t.Errorf("ID = %q", got.ID)
	}

	// Duplicate
	if err := reg.Register(wp); err == nil {
		t.Error("duplicate registration should fail")
	}

	// List
	list := reg.List()
	if len(list) != 1 {
		t.Errorf("List() = %d, want 1", len(list))
	}

	// Not found
	_, err = reg.Get("nonexistent")
	if err == nil {
		t.Error("Get nonexistent should fail")
	}
}

func TestWorkPackRegistryListByCategory(t *testing.T) {
	reg := cognitive.NewWorkPackRegistry()

	wp1 := validTestWorkPack()
	wp1.ID = "pack-a"
	wp1.Category = "productivity"

	wp2 := validTestWorkPack()
	wp2.ID = "pack-b"
	wp2.Category = "creative"

	wp3 := validTestWorkPack()
	wp3.ID = "pack-c"
	wp3.Category = "productivity"

	reg.Register(wp1)
	reg.Register(wp2)
	reg.Register(wp3)

	productivity := reg.ListByCategory("productivity")
	if len(productivity) != 2 {
		t.Errorf("ListByCategory(productivity) = %d, want 2", len(productivity))
	}

	creative := reg.ListByCategory("creative")
	if len(creative) != 1 {
		t.Errorf("ListByCategory(creative) = %d, want 1", len(creative))
	}
}

func TestWorkPackBudgetAlwaysZero(t *testing.T) {
	// All work packs should work with zero-cost budget
	packs := cognitive.DefaultWorkPacks()
	for _, wp := range packs {
		t.Run(wp.ID, func(t *testing.T) {
			// Verify no paid tools are required
			for _, tool := range wp.RequiredTools {
				if tool == "paid_api" || tool == "subscription" {
					t.Errorf("WorkPack %q requires paid tool %q", wp.ID, tool)
				}
			}
		})
	}
}

func TestWorkPackHasBeginnerGuide(t *testing.T) {
	packs := cognitive.DefaultWorkPacks()
	for _, wp := range packs {
		t.Run(wp.ID, func(t *testing.T) {
			if len(wp.BeginnerGuide) < 50 {
				t.Errorf("WorkPack %q beginner guide too short: %d chars", wp.ID, len(wp.BeginnerGuide))
			}
		})
	}
}

func TestWorkPackHasQualityChecks(t *testing.T) {
	packs := cognitive.DefaultWorkPacks()
	for _, wp := range packs {
		t.Run(wp.ID, func(t *testing.T) {
			if len(wp.QualityChecks) == 0 {
				t.Errorf("WorkPack %q has no quality checks", wp.ID)
			}
		})
	}
}

func validTestWorkPack() *cognitive.WorkPack {
	return &cognitive.WorkPack{
		ID:          "test-pack",
		Name:        "Test Pack",
		Description: "A test work pack for unit testing purposes",
		Version:     "1.0.0",
		Status:      cognitive.WorkPackComplete,
		Priority:    1,
		Category:    "test",
		IntakeSchema: []byte(`{"type":"object","properties":{"input":{"type":"string"}},"required":["input"],"additionalProperties":false}`),
		Workflow: []cognitive.WorkflowStep{
			{Name: "step1", Description: "First step", Risk: "low"},
		},
		RequiredTools: []string{"memory"},
		QualityChecks: []cognitive.QualityCheck{
			{Name: "check1", Description: "Basic check", Type: "llm", Criteria: "Must pass"},
		},
		DeliveryFormat:   "text",
		BeginnerGuide:    "This is a test work pack. It helps you test the work pack system. Just provide your input and I'll process it for you.",
		EstimatedMinutes: 5,
	}
}
