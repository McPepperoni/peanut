package main

import (
	"context"
	"strings"
	"testing"

	"peanut/internal/config"
)

func TestConfiguredRuntimeRejectsMissingRoleBeforeNativeAdapters(t *testing.T) {
	cfg := config.Config{Models: config.Models{Root: t.TempDir(), Threads: 1, CPUOnly: true}}
	_, err := newConfiguredCoordinator(context.Background(), cfg, nil)
	if err == nil || !strings.Contains(err.Error(), "role") || !strings.Contains(err.Error(), cfg.Models.Root) {
		t.Fatalf("error = %v, want role and model root", err)
	}
}
