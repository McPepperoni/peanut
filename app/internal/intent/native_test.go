package intent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"peanut/internal/llama"
)

const validPlanJSON = `{"version":1,"status":"execute","language":"en","steps":[{"device_id":"device.demo","action_id":"media.play","arguments":{"query":"Blue in Green"}}],"clarification":"","confidence":0.9}`

type fakeEngine struct {
	mu      sync.Mutex
	request llama.Request
	output  []byte
	closed  int
}

func (f *fakeEngine) Generate(_ context.Context, request llama.Request) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.request = request
	return f.output, nil
}

func (f *fakeEngine) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed++
	return nil
}

func TestNativeParserUsesSchemaAndValidatesPlan(t *testing.T) {
	engine := &fakeEngine{output: []byte(validPlanJSON)}
	parser := NewNativeParser(engine, 2, time.Second)
	plan, err := parser.Parse(context.Background(), "turn on demo", demoSnapshot())
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != StatusExecute {
		t.Fatalf("status = %q", plan.Status)
	}
	if engine.request.Schema == "" || engine.request.Prompt == "" {
		t.Fatal("native request missing schema or prompt")
	}
	if engine.request.Threads != 2 || engine.request.MaxTokens != 512 {
		t.Fatalf("request = %+v", engine.request)
	}
}

func TestNativeParserRejectsInventedCapability(t *testing.T) {
	parser := NewNativeParser(&fakeEngine{output: []byte(`{"version":1,"status":"execute","language":"en","steps":[{"device_id":"device.invented","action_id":"media.play","arguments":{"query":"jazz"}}],"clarification":"","confidence":0.9}`)}, 1, time.Second)
	if _, err := parser.Parse(context.Background(), "turn on demo", demoSnapshot()); err == nil {
		t.Fatal("accepted invented capability")
	}
}

func TestNativeParserCloseIsIdempotent(t *testing.T) {
	engine := &fakeEngine{output: []byte(validPlanJSON)}
	parser := NewNativeParser(engine, 1, time.Second)
	if err := parser.Close(); err != nil {
		t.Fatal(err)
	}
	if err := parser.Close(); err != nil {
		t.Fatal(err)
	}
	if engine.closed != 1 {
		t.Fatalf("engine close count = %d, want 1", engine.closed)
	}
}

func TestNativeParserHonorsTimeout(t *testing.T) {
	parser := NewNativeParser(waitingEngine{}, 1, time.Millisecond)
	if _, err := parser.Parse(context.Background(), "turn on demo", demoSnapshot()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want timeout", err)
	}
}

type waitingEngine struct{}

func (waitingEngine) Generate(ctx context.Context, _ llama.Request) ([]byte, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func (waitingEngine) Close() error { return nil }

func demoSnapshot() CapabilitySnapshot {
	return CapabilitySnapshot{Capabilities: []Capability{{
		ID: "capability.media", ProviderID: "provider.local", DeviceID: "device.demo",
		Type: "media", Name: "Demo", Actions: []ActionDefinition{{ID: "media.play", Arguments: map[string]ArgumentDefinition{
			"query": {Type: TypeString, Required: true},
		}}},
	}}}
}
