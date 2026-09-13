package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"peanut/internal/config"
	"peanut/internal/intent"
	"peanut/internal/models"
)

func TestConfiguredRuntimeRejectsMissingRoleBeforeNativeAdapters(t *testing.T) {
	cfg := config.Config{Models: config.Models{Root: t.TempDir(), Threads: 1, CPUOnly: true}}
	_, err := newConfiguredCoordinator(context.Background(), cfg, nil)
	if err == nil || !strings.Contains(err.Error(), "role") || !strings.Contains(err.Error(), cfg.Models.Root) {
		t.Fatalf("error = %v, want role and model root", err)
	}
}

func TestRunFailsBeforeAudioWhenRequiredModelMissing(t *testing.T) {
	err := runConfigured(context.Background(), config.Config{Models: config.Models{Root: t.TempDir(), Threads: 1, CPUOnly: true}}, nil)
	if err == nil || !strings.Contains(err.Error(), "role") {
		t.Fatalf("error = %v", err)
	}
}

func TestRuntimeRegistryReloadInvokesLiveSwapBoundary(t *testing.T) {
	root := t.TempDir()
	directory := filepath.Join(root, "intent", "local")
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "model.gguf"), []byte("model"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "model.json"), []byte(`{"id":"intent-live","role":"intent","runtime":"local","entry":"model.gguf"}`), 0644); err != nil {
		t.Fatal(err)
	}
	swaps := 0
	live := newReloadableModels(func(_ context.Context, snapshot models.Snapshot) (modelSet, error) {
		swaps++
		return modelSet{parser: runtimeTestParser{language: snapshot.Active[models.RoleIntent].ID}}, nil
	})
	registry := models.NewRegistry(root, nil, live.Swap)

	if _, err := registry.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	plan, err := live.Parse(context.Background(), "test", intent.CapabilitySnapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if swaps != 1 || plan.Language != "intent-live" {
		t.Fatalf("swaps = %d, delegated language = %q", swaps, plan.Language)
	}
}

type runtimeTestParser struct{ language string }

func (p runtimeTestParser) Parse(context.Context, string, intent.CapabilitySnapshot) (intent.ActionPlan, error) {
	return intent.ActionPlan{Language: p.language}, nil
}
