package intent

import (
	"strings"
	"testing"
)

func TestValidatePlanAcceptsCapabilityOnlyPlans(t *testing.T) {
	snapshot := testSnapshot()
	tests := []struct {
		name string
		plan ActionPlan
	}{
		{
			name: "single step music plan",
			plan: ActionPlan{Version: 1, Status: StatusExecute, Language: "en", Confidence: 0.9, Steps: []ActionRequest{
				{DeviceID: "device.living-room", ActionID: "media.play", Arguments: map[string]any{"query": "Blue in Green"}},
			}},
		},
		{
			name: "ordered multi-step plan",
			plan: ActionPlan{Version: 1, Status: StatusExecute, Language: "th-TH", Confidence: 0.8, Steps: []ActionRequest{
				{DeviceID: "device.living-room", ActionID: "media.volume", Arguments: map[string]any{"level": 0.25}},
				{DeviceID: "device.living-room", ActionID: "media.play", Arguments: map[string]any{"query": "แจ๊ส"}},
			}},
		},
		{
			name: "clarification",
			plan: ActionPlan{Version: 1, Status: StatusClarify, Language: "zh-CN", Clarification: "哪个房间？", Confidence: 0.4},
		},
		{
			name: "unknown",
			plan: ActionPlan{Version: 1, Status: StatusUnknown, Language: "ja", Confidence: 0.1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidatePlan(tt.plan, snapshot); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestValidatePlanRejectsInvalidCapabilityUse(t *testing.T) {
	snapshot := testSnapshot()
	base := ActionPlan{Version: 1, Status: StatusExecute, Language: "en", Confidence: 0.8, Steps: []ActionRequest{
		{DeviceID: "device.living-room", ActionID: "media.play", Arguments: map[string]any{"query": "Blue in Green"}},
	}}
	tests := []struct {
		name string
		edit func(*ActionPlan)
		want string
	}{
		{"unknown device", func(plan *ActionPlan) { plan.Steps[0].DeviceID = "device.missing" }, "unknown device"},
		{"unknown action", func(plan *ActionPlan) { plan.Steps[0].ActionID = "media.delete" }, "unknown action"},
		{"bad argument type", func(plan *ActionPlan) { plan.Steps[0].Arguments["query"] = 42.0 }, "argument query"},
		{"missing exactly-one argument", func(plan *ActionPlan) { delete(plan.Steps[0].Arguments, "query") }, "exactly one"},
		{"both exactly-one arguments", func(plan *ActionPlan) { plan.Steps[0].Arguments["uri"] = "https://example.com/song.mp3" }, "exactly one"},
		{"undeclared argument", func(plan *ActionPlan) { plan.Steps[0].Arguments["provider_payload"] = true }, "unknown argument"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := base
			plan.Steps = append([]ActionRequest(nil), base.Steps...)
			plan.Steps[0].Arguments = map[string]any{"query": "Blue in Green"}
			tt.edit(&plan)
			if err := ValidatePlan(plan, snapshot); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestValidatePlanAcceptsEitherExactlyOneArgument(t *testing.T) {
	for _, arguments := range []map[string]any{
		{"query": "Blue in Green"},
		{"uri": "https://example.com/song.mp3"},
	} {
		plan := ActionPlan{Version: 1, Status: StatusExecute, Language: "en", Confidence: 0.8, Steps: []ActionRequest{{
			DeviceID: "device.living-room", ActionID: "media.play", Arguments: arguments,
		}}}
		if err := ValidatePlan(plan, testSnapshot()); err != nil {
			t.Fatalf("arguments = %#v, error = %v", arguments, err)
		}
	}
}

func TestValidatePlanEnforcesEnvelopeAndStepBound(t *testing.T) {
	snapshot := testSnapshot()
	step := ActionRequest{DeviceID: "device.living-room", ActionID: "media.play", Arguments: map[string]any{"query": "x"}}
	tests := []struct {
		name string
		plan ActionPlan
	}{
		{"unsupported version", ActionPlan{Version: 2, Status: StatusUnknown, Language: "en"}},
		{"missing language", ActionPlan{Version: 1, Status: StatusUnknown}},
		{"execute without steps", ActionPlan{Version: 1, Status: StatusExecute, Language: "en"}},
		{"clarify without prompt", ActionPlan{Version: 1, Status: StatusClarify, Language: "en"}},
		{"clarify with steps", ActionPlan{Version: 1, Status: StatusClarify, Language: "en", Clarification: "Which?", Steps: []ActionRequest{step}}},
		{"unknown with steps", ActionPlan{Version: 1, Status: StatusUnknown, Language: "en", Steps: []ActionRequest{step}}},
		{"too many steps", ActionPlan{Version: 1, Status: StatusExecute, Language: "en", Steps: repeatStep(step, MaxPlanSteps+1)}},
		{"invalid confidence", ActionPlan{Version: 1, Status: StatusUnknown, Language: "en", Confidence: 1.1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidatePlan(tt.plan, snapshot); err == nil {
				t.Fatal("accepted invalid plan")
			}
		})
	}
}

func TestValidateSnapshotRejectsInvalidExactlyOneNames(t *testing.T) {
	for _, names := range [][]string{{"query", "query"}, {""}} {
		snapshot := testSnapshot()
		snapshot.Capabilities[0].Actions[0].ExactlyOneOf = names
		if err := ValidateSnapshot(snapshot); err == nil {
			t.Fatalf("accepted exactly-one names %#v", names)
		}
	}
}

func testSnapshot() CapabilitySnapshot {
	return CapabilitySnapshot{Capabilities: []Capability{{
		ID: "capability.living-room-media", ProviderID: "provider.local", DeviceID: "device.living-room",
		Type: "media", Name: "Living room speaker", Room: "Living room",
		Actions: []ActionDefinition{
			{ID: "media.play", Arguments: map[string]ArgumentDefinition{"query": {Type: TypeString}, "uri": {Type: TypeString}}, ExactlyOneOf: []string{"query", "uri"}},
			{ID: "media.volume", Arguments: map[string]ArgumentDefinition{"level": {Type: TypeNumber, Required: true}}},
		},
	}}}
}

func repeatStep(step ActionRequest, count int) []ActionRequest {
	steps := make([]ActionRequest, count)
	for i := range steps {
		steps[i] = step
	}
	return steps
}
