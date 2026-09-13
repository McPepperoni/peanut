package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"peanut/internal/audio"
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

func TestRuntimeClosesResourcesAfterCoordinatorStops(t *testing.T) {
	runtimeErr := errors.New("runtime failed")
	closeErr := errors.New("close failed")
	for _, test := range []struct {
		name     string
		serveErr error
		wantErr  error
	}{
		{name: "runtime error wins", serveErr: runtimeErr, wantErr: runtimeErr},
		{name: "close error follows clean shutdown", serveErr: http.ErrServerClosed, wantErr: closeErr},
	} {
		t.Run(test.name, func(t *testing.T) {
			cleanupStarted := make(chan struct{})
			releaseCleanup := make(chan struct{})
			capture := newRuntimeTestResource(closeErr)
			player := newRuntimeTestResource(nil)
			runCtx, cancel := context.WithCancel(context.Background())
			result := make(chan error, 1)
			go func() {
				err := runRuntimeProcesses(
					runCtx,
					cancel,
					func(ctx context.Context) error {
						<-ctx.Done()
						close(cleanupStarted)
						<-releaseCleanup
						return ctx.Err()
					},
					func() error { return test.serveErr },
					func() {},
				)
				if closeResourceErr := closeRuntimeResources(capture, player); err == nil {
					err = closeResourceErr
				}
				result <- err
			}()

			<-cleanupStarted
			select {
			case err := <-result:
				t.Fatalf("runtime returned before coordinator cleanup: %v", err)
			default:
			}
			for name, resource := range map[string]*runtimeTestResource{"capture": capture, "player": player} {
				select {
				case <-resource.closed:
					t.Fatalf("%s closed before coordinator cleanup", name)
				default:
				}
			}

			close(releaseCleanup)
			if err := <-result; !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}
			for name, resource := range map[string]*runtimeTestResource{"capture": capture, "player": player} {
				select {
				case <-resource.closed:
				default:
					t.Fatalf("%s was not closed", name)
				}
			}
		})
	}
}

type runtimeTestResource struct {
	closeErr error
	closed   chan struct{}
}

func newRuntimeTestResource(closeErr error) *runtimeTestResource {
	return &runtimeTestResource{closeErr: closeErr, closed: make(chan struct{})}
}

func (r *runtimeTestResource) Capture(context.Context) (<-chan audio.Frame, error) {
	return nil, nil
}

func (r *runtimeTestResource) Play(context.Context, audio.Audio) error { return nil }

func (r *runtimeTestResource) Close() error {
	close(r.closed)
	return r.closeErr
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
