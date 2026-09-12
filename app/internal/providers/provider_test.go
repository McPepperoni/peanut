package providers

import (
	"context"
	"errors"
	"testing"

	"peanut/internal/intent"
)

func TestRegistryAssemblesCapabilitySnapshot(t *testing.T) {
	registry := NewRegistry()
	provider := fakeProvider{
		descriptor: ProviderDescriptor{ID: "provider.test", Type: "compiled", Name: "Test provider"},
		capabilities: []intent.Capability{{
			ID: "capability.light", ProviderID: "provider.test", DeviceID: "device.light", Type: "light", Name: "Desk light",
			Actions: []intent.ActionDefinition{{ID: "power.set", Arguments: map[string]intent.ArgumentDefinition{"on": {Type: intent.TypeBoolean, Required: true}}}},
		}},
	}
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	registered, ok := registry.Provider("provider.test")
	if !ok || registered.Descriptor() != provider.descriptor {
		t.Fatalf("registered provider = %#v, %v", registered, ok)
	}
	snapshot, err := registry.Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Capabilities) != 1 || snapshot.Capabilities[0].DeviceID != "device.light" {
		t.Fatalf("snapshot = %#v", snapshot)
	}
	if err := registry.Register(provider); err == nil {
		t.Fatal("accepted duplicate provider ID")
	}
}

func TestRegistryRejectsInvalidDiscovery(t *testing.T) {
	registry := NewRegistry()
	provider := fakeProvider{
		descriptor:   ProviderDescriptor{ID: "provider.test", Type: "process", Name: "Test provider"},
		capabilities: []intent.Capability{{ID: "capability.bad", ProviderID: "provider.other", DeviceID: "device.bad", Type: "test", Name: "Bad"}},
	}
	if err := registry.Register(provider); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Snapshot(context.Background()); err == nil {
		t.Fatal("accepted capability owned by another provider")
	}
}

type fakeProvider struct {
	descriptor   ProviderDescriptor
	capabilities []intent.Capability
	err          error
}

func (p fakeProvider) Descriptor() ProviderDescriptor { return p.descriptor }
func (p fakeProvider) Discover(context.Context) ([]intent.Capability, error) {
	return p.capabilities, p.err
}
func (p fakeProvider) Execute(context.Context, intent.ActionRequest) (ActionResult, error) {
	return ActionResult{}, errors.New("not implemented")
}
