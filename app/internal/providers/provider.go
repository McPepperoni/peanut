package providers

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"peanut/internal/intent"
)

type ProviderDescriptor struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Name string `json:"name"`
}

type ActionResult struct {
	Data map[string]any `json:"data,omitempty"`
}

type CapabilityProvider interface {
	Descriptor() ProviderDescriptor
	Discover(context.Context) ([]intent.Capability, error)
	Execute(context.Context, intent.ActionRequest) (ActionResult, error)
}

type Registry struct {
	mu        sync.RWMutex
	providers []CapabilityProvider
	byID      map[string]CapabilityProvider
}

func NewRegistry() *Registry { return &Registry{byID: make(map[string]CapabilityProvider)} }

func (r *Registry) Register(provider CapabilityProvider) error {
	if provider == nil {
		return errors.New("provider is required")
	}
	descriptor := provider.Descriptor()
	if descriptor.ID == "" || descriptor.Type == "" || descriptor.Name == "" {
		return errors.New("provider stable ID, type, and name are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.byID[descriptor.ID]; exists {
		return fmt.Errorf("provider %q is already registered", descriptor.ID)
	}
	r.byID[descriptor.ID] = provider
	r.providers = append(r.providers, provider)
	return nil
}

func (r *Registry) Provider(id string) (CapabilityProvider, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	provider, ok := r.byID[id]
	return provider, ok
}

func (r *Registry) Snapshot(ctx context.Context) (intent.CapabilitySnapshot, error) {
	r.mu.RLock()
	providers := append([]CapabilityProvider(nil), r.providers...)
	r.mu.RUnlock()

	var snapshot intent.CapabilitySnapshot
	for _, provider := range providers {
		descriptor := provider.Descriptor()
		capabilities, err := provider.Discover(ctx)
		if err != nil {
			return intent.CapabilitySnapshot{}, fmt.Errorf("discover provider %q: %w", descriptor.ID, err)
		}
		for _, capability := range capabilities {
			if capability.ProviderID != descriptor.ID {
				return intent.CapabilitySnapshot{}, fmt.Errorf("capability %q belongs to provider %q, not %q", capability.ID, capability.ProviderID, descriptor.ID)
			}
		}
		snapshot.Capabilities = append(snapshot.Capabilities, capabilities...)
	}
	if err := intent.ValidateSnapshot(snapshot); err != nil {
		return intent.CapabilitySnapshot{}, err
	}
	return snapshot, nil
}
