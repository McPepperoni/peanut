package intent

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
)

const validPlanJSON = `{"version":1,"status":"execute","language":"en","steps":[{"device_id":"device.living-room","action_id":"media.play","arguments":{"query":"Blue in Green"}}],"clarification":"","confidence":0.9}`

type fakeInferenceRunner struct {
	output     []byte
	err        error
	wait       bool
	executable string
	args       []string
}

func (f *fakeInferenceRunner) Run(ctx context.Context, executable string, args []string) ([]byte, error) {
	f.executable = executable
	f.args = append([]string(nil), args...)
	if f.wait {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return f.output, f.err
}

func TestQwenParserPromptIncludesTranscriptAndCapabilities(t *testing.T) {
	runner := &fakeInferenceRunner{output: []byte(validPlanJSON)}
	parser := newQwenParser("llama-cli", "model.gguf", 4, time.Second, runner)

	if _, err := parser.Parse(context.Background(), "play jazz", testSnapshot()); err != nil {
		t.Fatal(err)
	}
	prompt := flagValue(t, runner.args, "--prompt")
	for _, want := range []string{"play jazz", "device.living-room", "media.play", "Living room speaker"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt does not contain %q: %s", want, prompt)
		}
	}
}

func TestQwenParserUsesCPUOnlyDeterministicNoThinkingFlags(t *testing.T) {
	runner := &fakeInferenceRunner{output: []byte(validPlanJSON)}
	parser := newQwenParser("llama-cli", "model.gguf", 3, time.Second, runner)

	if _, err := parser.Parse(context.Background(), "play jazz", testSnapshot()); err != nil {
		t.Fatal(err)
	}
	if runner.executable != "llama-cli" {
		t.Fatalf("executable = %q", runner.executable)
	}
	for flag, want := range map[string]string{
		"--model":                "model.gguf",
		"--threads":              "3",
		"--threads-batch":        "3",
		"--device":               "none",
		"--seed":                 "1",
		"--temp":                 "0",
		"--reasoning":            "off",
		"--chat-template-kwargs": `{"enable_thinking":false}`,
	} {
		if got := flagValue(t, runner.args, flag); got != want {
			t.Errorf("%s = %q, want %q", flag, got, want)
		}
	}
	if schema := flagValue(t, runner.args, "--json-schema"); !strings.Contains(schema, `"additionalProperties":false`) {
		t.Fatalf("JSON schema is not strict: %s", schema)
	}
}

func TestQwenParserSchemaAllowsCapabilityArguments(t *testing.T) {
	snapshot := CapabilitySnapshot{Capabilities: []Capability{{
		ID: "capability.media", ProviderID: "provider.local", DeviceID: "device.media",
		Type: "media", Name: "Media player", Actions: []ActionDefinition{{
			ID: "media.play", Arguments: map[string]ArgumentDefinition{
				"query":  {Type: TypeString, Required: true},
				"source": {Type: TypeString, Required: true},
			},
		}},
	}}}
	output := `{"version":1,"status":"execute","language":"en","steps":[{"device_id":"device.media","action_id":"media.play","arguments":{"query":"jazz","source":"library"}}],"clarification":"","confidence":0.9}`
	runner := &fakeInferenceRunner{output: []byte(output)}
	parser := newQwenParser("llama-cli", "model.gguf", 1, time.Second, runner)

	plan, err := parser.Parse(context.Background(), "play jazz from my library", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Steps[0].Arguments["query"] != "jazz" || plan.Steps[0].Arguments["source"] != "library" {
		t.Fatalf("arguments = %#v", plan.Steps[0].Arguments)
	}
	if schema := flagValue(t, runner.args, "--json-schema"); !strings.Contains(schema, `"arguments":{"type":"object","additionalProperties":true}`) {
		t.Fatalf("schema blocks capability arguments: %s", schema)
	}
	prompt := flagValue(t, runner.args, "--prompt")
	for _, name := range []string{`"query"`, `"source"`} {
		if !strings.Contains(prompt, name) {
			t.Fatalf("prompt does not contain argument %s: %s", name, prompt)
		}
	}
}

func TestQwenParserExtractsOnlyJSONPlan(t *testing.T) {
	runner := &fakeInferenceRunner{output: []byte("model output:\n```json\n" + validPlanJSON + "\n```\n")}
	parser := newQwenParser("llama-cli", "model.gguf", 1, time.Second, runner)

	plan, err := parser.Parse(context.Background(), "play jazz", testSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != StatusExecute || len(plan.Steps) != 1 {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestQwenParserRejectsMalformedOutput(t *testing.T) {
	runner := &fakeInferenceRunner{output: []byte("```json\n{not json}\n```")}
	parser := newQwenParser("llama-cli", "model.gguf", 1, time.Second, runner)

	if _, err := parser.Parse(context.Background(), "play jazz", testSnapshot()); err == nil || !strings.Contains(err.Error(), "JSON") {
		t.Fatalf("error = %v, want malformed JSON error", err)
	}
}

func TestQwenParserHonorsTimeout(t *testing.T) {
	runner := &fakeInferenceRunner{wait: true}
	parser := newQwenParser("llama-cli", "model.gguf", 1, time.Millisecond, runner)

	_, err := parser.Parse(context.Background(), "play jazz", testSnapshot())
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want deadline exceeded", err)
	}
}

func TestQwenParserRejectsInventedCapability(t *testing.T) {
	output := `{"version":1,"status":"execute","language":"en","steps":[{"device_id":"device.invented","action_id":"media.play","arguments":{"query":"jazz"}}],"clarification":"","confidence":0.9}`
	runner := &fakeInferenceRunner{output: []byte(output)}
	parser := newQwenParser("llama-cli", "model.gguf", 1, time.Second, runner)

	if _, err := parser.Parse(context.Background(), "play jazz", testSnapshot()); err == nil || !strings.Contains(err.Error(), "unknown device") {
		t.Fatalf("error = %v, want unknown device", err)
	}
}

func flagValue(t *testing.T, args []string, flag string) string {
	t.Helper()
	i := slices.Index(args, flag)
	if i < 0 || i+1 == len(args) {
		t.Fatalf("missing %s in %q", flag, args)
	}
	return args[i+1]
}
