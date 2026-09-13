package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sync"

	"peanut/assets"
	"peanut/internal/api"
	"peanut/internal/audio"
	"peanut/internal/audio/capture"
	"peanut/internal/audio/playback"
	"peanut/internal/config"
	"peanut/internal/intent"
	"peanut/internal/ml"
	"peanut/internal/ml/kws"
	"peanut/internal/ml/sherpa"
	"peanut/internal/ml/stt"
	"peanut/internal/ml/tts"
	"peanut/internal/ml/vad"
	"peanut/internal/models"
	"peanut/internal/pipeline"
	"peanut/internal/providers"
	"peanut/internal/storage/sqlite"
)

type modelSet struct {
	wake        kws.WakeDetector
	voice       vad.VAD
	transcriber stt.Transcriber
	parser      intent.IntentParser
	synthesizer tts.Synthesizer
}

type modelSetBuilder func(context.Context, models.Snapshot) (modelSet, error)

type reloadableModels struct {
	mu      sync.RWMutex
	build   modelSetBuilder
	current modelSet
}

func newReloadableModels(build modelSetBuilder) *reloadableModels {
	return &reloadableModels{build: build}
}

func (r *reloadableModels) Swap(ctx context.Context, snapshot models.Snapshot) error {
	next, err := r.build(ctx, snapshot)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.current = next
	r.mu.Unlock()
	return nil
}

func (r *reloadableModels) DetectVoice(ctx context.Context, frame audio.Frame) (vad.Result, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.current.voice == nil {
		return vad.Result{}, fmt.Errorf("runtime VAD model unavailable")
	}
	return r.current.voice.Detect(ctx, frame)
}

func (r *reloadableModels) Transcribe(ctx context.Context, input audio.Audio) (stt.Result, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.current.transcriber == nil {
		return stt.Result{}, fmt.Errorf("runtime STT model unavailable")
	}
	return r.current.transcriber.Transcribe(ctx, input)
}

func (r *reloadableModels) Parse(ctx context.Context, transcript string, snapshot intent.CapabilitySnapshot) (intent.ActionPlan, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.current.parser == nil {
		return intent.ActionPlan{}, fmt.Errorf("runtime intent model unavailable")
	}
	return r.current.parser.Parse(ctx, transcript, snapshot)
}

func (r *reloadableModels) Synthesize(ctx context.Context, text string) (tts.Result, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.current.synthesizer == nil {
		return tts.Result{}, fmt.Errorf("runtime TTS model unavailable")
	}
	return r.current.synthesizer.Synthesize(ctx, text)
}

type reloadableWake struct{ models *reloadableModels }

func (w reloadableWake) Detect(ctx context.Context, frame audio.Frame) (kws.Result, error) {
	w.models.mu.RLock()
	defer w.models.mu.RUnlock()
	if w.models.current.wake == nil {
		return kws.Result{}, fmt.Errorf("runtime KWS model unavailable")
	}
	return w.models.current.wake.Detect(ctx, frame)
}

func (w reloadableWake) Reset() error {
	w.models.mu.RLock()
	defer w.models.mu.RUnlock()
	if w.models.current.wake == nil {
		return fmt.Errorf("runtime KWS model unavailable")
	}
	return w.models.current.wake.Reset()
}

type reloadableVAD struct{ models *reloadableModels }

func (v reloadableVAD) Detect(ctx context.Context, frame audio.Frame) (vad.Result, error) {
	return v.models.DetectVoice(ctx, frame)
}

func (v reloadableVAD) Reset() error {
	v.models.mu.RLock()
	defer v.models.mu.RUnlock()
	if v.models.current.voice == nil {
		return fmt.Errorf("runtime VAD model unavailable")
	}
	return v.models.current.voice.Reset()
}

func runConfigured(ctx context.Context, cfg config.Config, db *sqlite.DB) (err error) {
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	runtime, err := newConfiguredRuntime(runCtx, cfg, db)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := closeRuntimeResources(runtime.capture, runtime.player); err == nil {
			err = closeErr
		}
	}()
	server := api.NewServer(db, runtime.homeAssistant, runtime.modelRegistry)
	address, err := server.Address(runCtx)
	if err != nil {
		return err
	}
	httpServer := &http.Server{Addr: address, Handler: server.Handler()}
	runErrors := make(chan error, 2)
	go func() { runErrors <- runtime.coordinator.Run(runCtx) }()
	go func() { runErrors <- httpServer.ListenAndServe() }()
	var runErr error
	select {
	case runErr = <-runErrors:
	case <-runCtx.Done():
		runErr = runCtx.Err()
	}
	cancel()
	_ = httpServer.Shutdown(context.Background())
	if errors.Is(runErr, http.ErrServerClosed) {
		return nil
	}
	return runErr
}

func closeRuntimeResources(capture audio.Capture, player audio.Player) error {
	var closeErrs []error
	for _, resource := range []any{capture, player} {
		if closer, ok := resource.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				closeErrs = append(closeErrs, err)
			}
		}
	}
	return errors.Join(closeErrs...)
}

func newConfiguredCoordinator(ctx context.Context, cfg config.Config, db *sqlite.DB) (*pipeline.Coordinator, error) {
	runtime, err := newConfiguredRuntime(ctx, cfg, db)
	if err != nil {
		return nil, err
	}
	return runtime.coordinator, nil
}

type configuredRuntime struct {
	coordinator   *pipeline.Coordinator
	homeAssistant *providers.HomeAssistantProvider
	modelRegistry *models.Registry
	capture       audio.Capture
	player        audio.Player
}

func newConfiguredRuntime(ctx context.Context, cfg config.Config, db *sqlite.DB) (*configuredRuntime, error) {
	var modelStore models.Store
	if db != nil {
		modelStore = sqlite.NewModelStore(db)
	}
	liveModels := newReloadableModels(buildModelSet(cfg))
	modelRegistry := models.NewRegistry(cfg.Models.Root, modelStore, liveModels.Swap)
	if _, err := modelRegistry.Reload(ctx); err != nil {
		return nil, fmt.Errorf("scan model roles at %s: %w", cfg.Models.Root, err)
	}
	acknowledgement, err := assets.Acknowledgement()
	if err != nil {
		return nil, fmt.Errorf("load acknowledgement: %w", err)
	}
	providerRegistry := providers.NewRegistry()
	homeAssistant := providers.NewHomeAssistantProvider(db, nil)
	if err := providerRegistry.Register(homeAssistant); err != nil {
		return nil, err
	}
	capabilities, err := providerRegistry.Snapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("discover capabilities: %w", err)
	}
	pcfg := pipeline.DefaultConfig()
	pcfg.PreRollFrames = max(1, cfg.Audio.PreRollMilliseconds*audio.SampleRate/(1000*audio.FrameSamples))
	pcfg.PreRollSamples = pcfg.PreRollFrames * audio.FrameSamples
	pcfg.VADThreshold = cfg.Audio.VADThreshold
	pcfg.AcknowledgementTail = cfg.Audio.AcknowledgementTail
	pcfg.PlaybackTail = cfg.Audio.PlaybackTail
	pcfg.NoSpeechTimeout = cfg.Audio.NoSpeechTimeout
	pcfg.MaximumCommandDuration = cfg.Audio.MaximumCommandDuration
	systemCapture := capture.SystemCapture{}
	systemPlayer := playback.SystemPlayer{}
	coordinator, err := pipeline.NewCoordinator(pcfg, pipeline.Dependencies{
		Capture:         systemCapture,
		Player:          systemPlayer,
		Acknowledgement: acknowledgement,
		WakeDetector:    reloadableWake{models: liveModels},
		VAD:             reloadableVAD{models: liveModels},
		Transcriber:     liveModels,
		IntentParser:    liveModels,
		Synthesizer:     liveModels,
		Registry:        providerRegistry,
		Capabilities:    capabilities,
	})
	if err != nil {
		return nil, err
	}
	return &configuredRuntime{coordinator: coordinator, homeAssistant: homeAssistant, modelRegistry: modelRegistry, capture: systemCapture, player: systemPlayer}, nil
}

func buildModelSet(cfg config.Config) modelSetBuilder {
	return func(_ context.Context, snapshot models.Snapshot) (modelSet, error) {
		required, err := requiredProfiles(cfg.Models.Root, snapshot)
		if err != nil {
			return modelSet{}, err
		}
		manifest := ml.Manifest{
			Paths: ml.Paths{
				KWS:     modelDirectory(cfg.Models.Root, required[models.RoleKWS]),
				VAD:     modelEntry(cfg.Models.Root, required[models.RoleVAD]),
				STT:     modelDirectory(cfg.Models.Root, required[models.RoleSTT]),
				Speaker: modelEntry(cfg.Models.Root, required[models.RoleSpeaker]),
				TTS:     modelDirectory(cfg.Models.Root, required[models.RoleTTS]),
			},
			Provider: ml.CPUProvider,
			Threads:  cfg.Models.Threads,
		}
		wake, err := sherpa.NewWakeDetector(manifest)
		if err != nil {
			return modelSet{}, fmt.Errorf("create wake detector: %w", err)
		}
		voice, err := sherpa.NewVAD(manifest)
		if err != nil {
			return modelSet{}, fmt.Errorf("create VAD: %w", err)
		}
		transcriber, err := sherpa.NewTranscriber(manifest)
		if err != nil {
			return modelSet{}, fmt.Errorf("create transcriber: %w", err)
		}
		synthesizer, err := sherpa.NewSynthesizer(manifest)
		if err != nil {
			return modelSet{}, fmt.Errorf("create synthesizer: %w", err)
		}
		intentProfile := required[models.RoleIntent]
		return modelSet{
			wake:        wake,
			voice:       voice,
			transcriber: transcriber,
			parser:      intent.NewModelParser(intentProfile.Runtime, modelEntry(cfg.Models.Root, intentProfile), cfg.Models.Threads, cfg.HomeAssistant.Timeout),
			synthesizer: synthesizer,
		}, nil
	}
}

func requiredProfiles(root string, snapshot models.Snapshot) (map[models.Role]models.Profile, error) {
	required := make(map[models.Role]models.Profile, 6)
	for _, role := range []models.Role{models.RoleKWS, models.RoleVAD, models.RoleSTT, models.RoleSpeaker, models.RoleTTS, models.RoleIntent} {
		profile, ok := snapshot.Active[role]
		if !ok {
			path, detail := filepath.Join(root, string(role)), "missing"
			for _, candidate := range snapshot.Profiles {
				if candidate.Role == role {
					path = filepath.Join(root, filepath.FromSlash(candidate.Path), filepath.FromSlash(candidate.Entry))
					detail = candidate.Error
					break
				}
			}
			return nil, fmt.Errorf("load %s model: role %q at %s is invalid: %s", role, role, path, detail)
		}
		required[role] = profile
	}
	return required, nil
}

func modelDirectory(root string, profile models.Profile) string {
	return filepath.Join(root, filepath.FromSlash(profile.Path))
}

func modelEntry(root string, profile models.Profile) string {
	return filepath.Join(modelDirectory(root, profile), filepath.FromSlash(profile.Entry))
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}
