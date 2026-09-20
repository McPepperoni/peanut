package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"peanut/internal/audio"
	"peanut/internal/config"
	"peanut/internal/intent"
	"peanut/internal/llama"
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
					func() error { return nil },
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

func TestRuntimeWaitsForHTTPServerDrain(t *testing.T) {
	serverReleased := make(chan struct{})
	finishDrain := make(chan struct{})
	result := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		result <- runRuntimeProcesses(
			ctx,
			cancel,
			func(context.Context) error { return nil },
			func() error {
				<-serverReleased
				<-finishDrain
				return http.ErrServerClosed
			},
			func() error {
				close(serverReleased)
				return nil
			},
		)
	}()
	<-serverReleased
	select {
	case err := <-result:
		t.Fatalf("runtime returned before HTTP drain: %v", err)
	default:
	}
	close(finishDrain)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestRunHTTPServerWaitsForShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	serverReleased := make(chan struct{})
	finishDrain := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- runHTTPServer(ctx, func() error {
			<-serverReleased
			<-finishDrain
			return http.ErrServerClosed
		}, func() error {
			close(serverReleased)
			return nil
		})
	}()
	cancel()
	<-serverReleased
	select {
	case err := <-result:
		t.Fatalf("server returned before drain: %v", err)
	default:
	}
	close(finishDrain)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

func TestHTTPShutdownForcesCloseAfterDeadline(t *testing.T) {
	handlerStarted := make(chan struct{})
	handlerDone := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(handlerStarted)
		<-r.Context().Done()
		close(handlerDone)
	}))
	defer server.Close()
	requestDone := make(chan struct{})
	go func() {
		_, _ = server.Client().Get(server.URL)
		close(requestDone)
	}()
	<-handlerStarted

	err := shutdownHTTPServer(server.Config, 10*time.Millisecond)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown error = %v", err)
	}
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("forced close did not cancel handler")
	}
	<-requestDone
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

func TestBuildModelSetUsesFlatIntentModel(t *testing.T) {
	root := t.TempDir()
	modelPath := filepath.Join(root, "functiongemma.gguf")
	if err := os.WriteFile(modelPath, []byte("model"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Models: config.Models{Root: root, IntentModel: "functiongemma.gguf", Threads: 2}, HomeAssistant: config.HomeAssistant{Timeout: time.Second}}
	engine := &runtimeTestEngine{}
	previous := openLlama
	defer func() { openLlama = previous }()
	var gotPath string
	openLlama = func(_ context.Context, path string, threads int) (llama.Engine, error) {
		gotPath = path
		if threads != 2 {
			t.Fatalf("threads = %d, want 2", threads)
		}
		return engine, nil
	}

	set, err := buildModelSet(cfg, models.RoleIntent)(context.Background(), models.Snapshot{})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != modelPath {
		t.Fatalf("model path = %q, want %q", gotPath, modelPath)
	}
	if _, ok := set.parser.(*intent.NativeParser); !ok {
		t.Fatalf("parser = %T, want native parser", set.parser)
	}
	if err := closeModelSet(set); err != nil {
		t.Fatal(err)
	}
	if engine.closed != 1 {
		t.Fatalf("engine close count = %d, want 1", engine.closed)
	}
}

func TestRuntimeSwapRejectsNativeIntentOpenBeforePublication(t *testing.T) {
	root := t.TempDir()
	modelPath := filepath.Join(root, "functiongemma.gguf")
	if err := os.WriteFile(modelPath, []byte("model"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Models: config.Models{Root: root, IntentModel: "functiongemma.gguf", Threads: 1}, HomeAssistant: config.HomeAssistant{Timeout: time.Second}}
	previousOpen := openLlama
	defer func() { openLlama = previousOpen }()
	openLlama = func(context.Context, string, int) (llama.Engine, error) {
		return nil, errors.New("native open failed")
	}
	live := newReloadableModels(buildModelSet(cfg, models.RoleIntent))
	live.current = modelSet{parser: runtimeTestParser{language: "old"}}

	err := live.Swap(context.Background(), models.Snapshot{})
	if err == nil || !strings.Contains(err.Error(), "load intent model") {
		t.Fatalf("swap error = %v", err)
	}
	plan, parseErr := live.Parse(context.Background(), "test", intent.CapabilitySnapshot{})
	if parseErr != nil || plan.Language != "old" {
		t.Fatalf("previous parser not preserved: plan = %#v, error = %v", plan, parseErr)
	}
}

func TestRuntimeSwapClosesFailedNativeCandidate(t *testing.T) {
	root := t.TempDir()
	modelPath := filepath.Join(root, "functiongemma.gguf")
	if err := os.WriteFile(modelPath, []byte("model"), 0644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{Models: config.Models{Root: root, IntentModel: "functiongemma.gguf", Threads: 1}, HomeAssistant: config.HomeAssistant{Timeout: time.Second}}
	candidate := &runtimeTestEngine{}
	previousOpen := openLlama
	defer func() { openLlama = previousOpen }()
	openLlama = func(context.Context, string, int) (llama.Engine, error) { return candidate, nil }
	live := newReloadableModels(buildModelSet(cfg, models.RoleIntent, models.RoleSTT))
	previous := &runtimeTestEngine{output: []byte(`{"version":1,"status":"unknown","language":"old","steps":[],"clarification":"","confidence":0.5}`)}
	live.current = modelSet{parser: intent.NewNativeParser(previous, 1, time.Second)}
	defer live.Close()

	err := live.Swap(context.Background(), models.Snapshot{Active: map[models.Role]models.Profile{
		models.RoleSTT: {Role: models.RoleSTT, Path: "stt", Entry: "model.onnx", Valid: true},
	}})
	if err == nil {
		t.Fatal("accepted failed replacement")
	}
	if candidate.closed != 1 {
		t.Fatalf("candidate close count = %d, want 1", candidate.closed)
	}
	if previous.closed != 0 {
		t.Fatalf("previous close count = %d, want 0", previous.closed)
	}
	plan, parseErr := live.Parse(context.Background(), "test", intent.CapabilitySnapshot{})
	if parseErr != nil || plan.Language != "old" {
		t.Fatalf("previous parser not preserved: plan = %#v, error = %v", plan, parseErr)
	}
}

type runtimeTestEngine struct {
	output []byte
	closed int
}

func (e *runtimeTestEngine) Generate(context.Context, llama.Request) ([]byte, error) {
	return e.output, nil
}

func (e *runtimeTestEngine) Close() error {
	e.closed++
	return nil
}

type runtimeTestParser struct{ language string }

func (p runtimeTestParser) Parse(context.Context, string, intent.CapabilitySnapshot) (intent.ActionPlan, error) {
	return intent.ActionPlan{Language: p.language}, nil
}
