package intent

import (
	"strings"
	"testing"
)

func TestDecodePlanUsesStrictJSON(t *testing.T) {
	data := []byte(`{"version":1,"status":"execute","language":"ko","steps":[{"device_id":"device.living-room","action_id":"media.play","arguments":{"query":"재즈"}}],"clarification":"","confidence":0.9}`)
	plan, err := DecodePlan(data)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Language != "ko" || len(plan.Steps) != 1 || plan.Steps[0].Arguments["query"] != "재즈" {
		t.Fatalf("decoded plan = %#v", plan)
	}

	for _, invalid := range []string{
		`{"version":1,"status":"unknown","language":"en","steps":[],"clarification":"","confidence":0,"extra":true}`,
		`{"version":1,"status":"unknown","language":"en","steps":[],"clarification":"","confidence":0} {}`,
		`{"version":1,"status":"unknown","language":"en"}`,
	} {
		if _, err := DecodePlan([]byte(invalid)); err == nil || !strings.Contains(err.Error(), "plan") {
			t.Fatalf("DecodePlan(%q) error = %v", invalid, err)
		}
	}
}
