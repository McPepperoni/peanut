package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"peanut/assets"
	"peanut/internal/api"
	"peanut/internal/audio"
	"peanut/internal/audio/capture"
	"peanut/internal/audio/playback"
	"peanut/internal/config"
	"peanut/internal/intent"
	"peanut/internal/logging"
	"peanut/internal/ml"
	"peanut/internal/ml/kws"
	"peanut/internal/ml/sherpa"
	mlspeaker "peanut/internal/ml/speaker"
	"peanut/internal/ml/stt"
	"peanut/internal/ml/tts"
	"peanut/internal/ml/vad"
	"peanut/internal/models"
	"peanut/internal/pipeline"
	"peanut/internal/providers"
	speakerstore "peanut/internal/speaker"
	"peanut/internal/storage/sqlite"
)

const gracefulHTTPShutdownTimeout = 5 * time.Second

type modelSet struct {
	wake        kws.WakeDetector
	voice       vad.VAD
	transcriber stt.Transcriber
	speaker     mlspeaker.SpeakerIdentifier
	parser      intent.IntentParser
	synthesizer tts.Synthesizer
}

type modelSetBuilder func(context.Context, models.Snapshot) (modelSet, error)

type reloadableModels struct {
	mu      sync.RWMutex
	build   modelSetBuilder
	logger  *slog.Logger
	current modelSet
}

func newReloadableModels(build modelSetBuilder) *reloadableModels {
	return newReloadableModelsWithLogger(build, logging.Nop())
}

func newReloadableModelsWithLogger(build modelSetBuilder, logger *slog.Logger) *reloadableModels {
	return &reloadableModels{build: build, logger: logging.Normalize(logger)}
}

func (r *reloadableModels) Swap(ctx context.Context, snapshot models.Snapshot) error {
	r.logger.Info("model.reload", "component", "model", "status", "started")
	next, err := r.build(ctx, snapshot)
	if err != nil {
		r.logger.Error("model.reload", "component", "model", "status", "failed", "error_type", "operation_failed")
		return err
	}
	r.mu.Lock()
	previous := r.current
	r.current = next
	r.mu.Unlock()
	_ = closeModelSet(previous)
	r.logger.Info("model.reload", "component", "model", "status", "succeeded")
	return nil
}

func (r *reloadableModels) Close() error {
	r.mu.Lock()
	previous := r.current
	r.current = modelSet{}
	r.mu.Unlock()
	return closeModelSet(previous)
}

func closeModelSet(set modelSet) error {
	var errs []error
	for _, resource := range []any{set.wake, set.voice, set.transcriber, set.speaker, set.parser, set.synthesizer} {
		if closer, ok := resource.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
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

func (r *reloadableModels) Identify(ctx context.Context, input audio.Audio) mlspeaker.Result {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.current.speaker == nil {
		return mlspeaker.Result{Err: fmt.Errorf("runtime speaker model unavailable")}
	}
	return r.current.speaker.Identify(ctx, input)
}

func (r *reloadableModels) Embed(ctx context.Context, input audio.Audio) ([]float32, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	embedder, ok := r.current.speaker.(speakerstore.Embedder)
	if !ok {
		return nil, fmt.Errorf("runtime speaker embedder unavailable")
	}
	return embedder.Embed(ctx, input)
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

type runtimeSpeaker struct {
	models *reloadableModels
	store  *speakerstore.Store
}

func (s runtimeSpeaker) Embed(ctx context.Context, input audio.Audio) ([]float32, error) {
	return s.models.Embed(ctx, input)
}

func (s runtimeSpeaker) Identify(ctx context.Context, input audio.Audio) mlspeaker.Result {
	embedding, err := s.models.Embed(ctx, input)
	if err != nil {
		return mlspeaker.Result{Err: err}
	}
	if s.store == nil {
		return mlspeaker.Result{Err: fmt.Errorf("speaker store unavailable")}
	}
	id, score, err := s.store.Match(ctx, embedding, 0.75)
	return mlspeaker.Result{ID: id, Score: score, Err: err}
}

func runConfigured(ctx context.Context, cfg config.Config, db *sqlite.DB) (err error) {
	return runConfiguredWithLogger(ctx, cfg, db, logging.Nop())
}

func runConfiguredWithLogger(ctx context.Context, cfg config.Config, db *sqlite.DB, logger *slog.Logger) (err error) {
	logger = logging.Normalize(logger)
	logger.Info("runtime.start", "component", "runtime", "status", "starting")
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	runtime, err := newConfiguredRuntimeWithLogger(runCtx, cfg, db, logger)
	if err != nil {
		return err
	}
	defer func() {
		closeErr := errors.Join(closeRuntimeResources(runtime.capture, runtime.player), runtime.modelSet.Close())
		if closeErr != nil && err == nil {
			err = closeErr
		}
	}()
	server := api.NewServerWithLogger(db, runtime.homeAssistant, runtime.modelRegistry, logger)
	address, err := server.Address(runCtx)
	if err != nil {
		return err
	}
	server.SetBoundAddress(address)
	httpServer := &http.Server{Addr: address, Handler: server.Handler()}
	err = runRuntimeProcesses(
		runCtx,
		cancel,
		runtime.coordinator.Run,
		httpServer.ListenAndServe,
		func() error { return shutdownHTTPServer(httpServer, gracefulHTTPShutdownTimeout) },
	)
	if err != nil {
		logger.Error("runtime.stop", "component", "runtime", "status", "failed", "error_type", "operation_failed")
		return err
	}
	logger.Info("runtime.stop", "component", "runtime", "status", "stopped")
	return nil
}

func runRuntimeProcesses(ctx context.Context, cancel context.CancelFunc, runCoordinator func(context.Context) error, serve func() error, shutdownServer func() error) error {
	coordinatorDone := make(chan error, 1)
	serverDone := make(chan error, 1)
	go func() { coordinatorDone <- runCoordinator(ctx) }()
	go func() { serverDone <- serve() }()

	var runErr error
	coordinatorStopped := false
	serverStopped := false
	select {
	case runErr = <-coordinatorDone:
		coordinatorStopped = true
	case runErr = <-serverDone:
		serverStopped = true
	case <-ctx.Done():
		runErr = ctx.Err()
	}
	cancel()
	shutdownErr := shutdownServer()
	var coordinatorErr, serverErr error
	if !coordinatorStopped {
		coordinatorErr = <-coordinatorDone
	}
	if !serverStopped {
		serverErr = <-serverDone
	}
	for _, err := range []error{runErr, coordinatorErr, serverErr} {
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	}
	return shutdownErr
}

func runHTTPServer(ctx context.Context, serve func() error, shutdown func() error) error {
	done := make(chan error, 1)
	go func() { done <- serve() }()
	select {
	case err := <-done:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownErr := shutdown()
		serveErr := <-done
		if shutdownErr != nil {
			return shutdownErr
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			return serveErr
		}
		return nil
	}
}

func shutdownHTTPServer(server *http.Server, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := server.Shutdown(ctx); err != nil {
		return errors.Join(err, server.Close())
	}
	return nil
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
	modelSet      *reloadableModels
	speakerStore  *speakerstore.Store
	capture       audio.Capture
	player        audio.Player
	logger        *slog.Logger
}

func newConfiguredRuntime(ctx context.Context, cfg config.Config, db *sqlite.DB) (*configuredRuntime, error) {
	return newConfiguredRuntimeWithLogger(ctx, cfg, db, logging.Nop())
}

func newConfiguredRuntimeWithLogger(ctx context.Context, cfg config.Config, db *sqlite.DB, logger *slog.Logger) (*configuredRuntime, error) {
	logger = logging.Normalize(logger)
	liveModels := newReloadableModelsWithLogger(buildModelSetWithLogger(cfg, logger), logger)
	ready := false
	defer func() {
		if !ready {
			_ = liveModels.Close()
		}
	}()
	modelRegistry := newRuntimeModelRegistry(cfg, db, liveModels)
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
	pcfg.DebugAudio = cfg.Audio.DebugAudio
	pcfg.DebugAudioPath = cfg.Audio.DebugAudioPath
	systemCapture := capture.SystemCapture{Device: cfg.Audio.InputDevice}
	systemPlayer := playback.SystemPlayer{Device: cfg.Audio.OutputDevice}
	speakerStore := speakerstore.NewSpeakerStore(db)
	coordinator, err := pipeline.NewCoordinator(pcfg, pipeline.Dependencies{
		Capture:         systemCapture,
		Player:          systemPlayer,
		Acknowledgement: acknowledgement,
		WakeDetector:    reloadableWake{models: liveModels},
		VAD:             reloadableVAD{models: liveModels},
		Transcriber:     liveModels,
		IntentParser:    liveModels,
		Synthesizer:     liveModels,
		Speaker:         runtimeSpeaker{models: liveModels, store: speakerStore},
		Registry:        providerRegistry,
		Capabilities:    capabilities,
		Logger:          logger,
	})
	if err != nil {
		return nil, err
	}
	ready = true
	return &configuredRuntime{coordinator: coordinator, homeAssistant: homeAssistant, modelRegistry: modelRegistry, modelSet: liveModels, speakerStore: speakerStore, capture: systemCapture, player: systemPlayer, logger: logger}, nil
}

func newCommandRuntime(ctx context.Context, cfg config.Config, db *sqlite.DB, command string) (*configuredRuntime, error) {
	return newCommandRuntimeWithLogger(ctx, cfg, db, command, logging.Nop())
}

func newCommandRuntimeWithLogger(ctx context.Context, cfg config.Config, db *sqlite.DB, command string, logger *slog.Logger) (*configuredRuntime, error) {
	logger = logging.Normalize(logger)
	var roles []models.Role
	switch command {
	case "speak":
		roles = []models.Role{models.RoleTTS}
	case "transcribe":
		roles = []models.Role{models.RoleSTT}
	case "enroll":
		roles = []models.Role{models.RoleSpeaker}
	default:
		return nil, fmt.Errorf("unsupported command runtime %q", command)
	}
	build := func(buildCtx context.Context, snapshot models.Snapshot) (modelSet, error) {
		selected := append([]models.Role(nil), roles...)
		if command == "transcribe" {
			if _, ok := snapshot.Active[models.RoleSpeaker]; ok {
				selected = append(selected, models.RoleSpeaker)
			}
		}
		set, err := buildModelSetWithLogger(cfg, logger, selected...)(buildCtx, snapshot)
		if err != nil && command == "transcribe" && len(selected) > len(roles) {
			logger.Warn("model.fallback", "component", "model", "role", string(models.RoleSpeaker), "status", "fallback")
			return buildModelSetWithLogger(cfg, logger, roles...)(buildCtx, snapshot)
		}
		return set, err
	}
	liveModels := newReloadableModelsWithLogger(build, logger)
	ready := false
	defer func() {
		if !ready {
			_ = liveModels.Close()
		}
	}()
	modelRegistry := newRuntimeModelRegistry(cfg, db, liveModels)
	if _, err := modelRegistry.Reload(ctx); err != nil {
		return nil, fmt.Errorf("scan model roles at %s: %w", cfg.Models.Root, err)
	}
	systemCapture := capture.SystemCapture{Device: cfg.Audio.InputDevice}
	systemPlayer := playback.SystemPlayer{Device: cfg.Audio.OutputDevice}
	ready = true
	return &configuredRuntime{
		modelRegistry: modelRegistry,
		modelSet:      liveModels,
		speakerStore:  speakerstore.NewSpeakerStore(db),
		capture:       systemCapture,
		player:        systemPlayer,
		logger:        logger,
	}, nil
}

func newRuntimeModelRegistry(cfg config.Config, db *sqlite.DB, owner *reloadableModels) *models.Registry {
	var modelStore models.Store
	if db != nil {
		modelStore = sqlite.NewModelStore(db)
	}
	return models.NewRegistry(cfg.Models.Root, modelStore, owner.Swap)
}

func buildModelSet(cfg config.Config, roles ...models.Role) modelSetBuilder {
	return func(_ context.Context, snapshot models.Snapshot) (modelSet, error) {
		required, err := requiredProfiles(cfg.Models.Root, snapshot, roles...)
		if err != nil {
			return modelSet{}, err
		}
		manifest := ml.Manifest{
			Provider: ml.CPUProvider,
			Threads:  cfg.Models.Threads,
		}
		if profile, ok := required[models.RoleKWS]; ok {
			manifest.Paths.KWS = modelDirectory(cfg.Models.Root, profile)
		}
		if profile, ok := required[models.RoleVAD]; ok {
			manifest.Paths.VAD = modelEntry(cfg.Models.Root, profile)
		}
		if profile, ok := required[models.RoleSTT]; ok {
			manifest.Paths.STT = modelDirectory(cfg.Models.Root, profile)
		}
		if profile, ok := required[models.RoleSpeaker]; ok {
			manifest.Paths.Speaker = modelEntry(cfg.Models.Root, profile)
		}
		if profile, ok := required[models.RoleTTS]; ok {
			manifest.Paths.TTS = modelDirectory(cfg.Models.Root, profile)
		}
		var intentExecutable string
		if intentProfile, ok := required[models.RoleIntent]; ok {
			intentExecutable, err = resolveIntentExecutable(intentProfile.Runtime)
			if err != nil {
				return modelSet{}, err
			}
		}
		var set modelSet
		if _, ok := required[models.RoleKWS]; ok {
			set.wake, err = sherpa.NewWakeDetector(manifest)
			if err != nil {
				return modelSet{}, fmt.Errorf("create wake detector: %w", err)
			}
		}
		if _, ok := required[models.RoleVAD]; ok {
			set.voice, err = sherpa.NewVAD(manifest, sherpa.WithVADThreshold(cfg.Audio.VADThreshold))
			if err != nil {
				_ = closeModelSet(set)
				return modelSet{}, fmt.Errorf("create VAD: %w", err)
			}
		}
		if _, ok := required[models.RoleSTT]; ok {
			set.transcriber, err = sherpa.NewTranscriber(manifest)
			if err != nil {
				_ = closeModelSet(set)
				return modelSet{}, fmt.Errorf("create transcriber: %w", err)
			}
		}
		if _, ok := required[models.RoleSpeaker]; ok {
			set.speaker, err = sherpa.NewSpeakerIdentifier(manifest)
			if err != nil {
				_ = closeModelSet(set)
				return modelSet{}, fmt.Errorf("create speaker identifier: %w", err)
			}
		}
		if _, ok := required[models.RoleTTS]; ok {
			set.synthesizer, err = sherpa.NewSynthesizer(manifest)
			if err != nil {
				_ = closeModelSet(set)
				return modelSet{}, fmt.Errorf("create synthesizer: %w", err)
			}
		}
		if intentProfile, ok := required[models.RoleIntent]; ok {
			set.parser = intent.NewModelParser(intentExecutable, modelEntry(cfg.Models.Root, intentProfile), cfg.Models.Threads, cfg.HomeAssistant.Timeout)
		}
		return set, nil
	}
}

func buildModelSetWithLogger(cfg config.Config, logger *slog.Logger, roles ...models.Role) modelSetBuilder {
	logger = logging.Normalize(logger)
	base := buildModelSet(cfg, roles...)
	if len(roles) == 0 {
		roles = []models.Role{models.RoleKWS, models.RoleVAD, models.RoleSTT, models.RoleSpeaker, models.RoleTTS, models.RoleIntent}
	}
	return func(ctx context.Context, snapshot models.Snapshot) (modelSet, error) {
		started := time.Now()
		for _, role := range roles {
			logger.Info("model.load", "component", "model", "role", string(role), "status", "started")
		}
		set, err := base(ctx, snapshot)
		status := "succeeded"
		level := slog.LevelInfo
		if err != nil {
			status = "failed"
			level = slog.LevelError
		}
		for _, role := range roles {
			logger.LogAttrs(ctx, level, "model.load", slog.String("component", "model"), slog.String("role", string(role)), slog.String("status", status), slog.Int64("duration_ms", time.Since(started).Milliseconds()))
		}
		return set, err
	}
}

func intentExecutableName(runtimeName string) (string, error) {
	switch runtimeName {
	case "local", "llama.cpp", "llama-cli":
		return "llama-cli", nil
	default:
		if filepath.IsAbs(runtimeName) || strings.ContainsAny(runtimeName, `/\\`) {
			return runtimeName, nil
		}
		return "", fmt.Errorf("unsupported intent runtime %q", runtimeName)
	}
}

func resolveIntentExecutable(runtimeName string) (string, error) {
	executable, err := intentExecutableName(runtimeName)
	if err != nil {
		return "", err
	}
	resolved, err := exec.LookPath(executable)
	if err != nil {
		return "", fmt.Errorf("resolve intent runtime %q: %w", runtimeName, err)
	}
	return resolved, nil
}

func requiredProfiles(root string, snapshot models.Snapshot, roles ...models.Role) (map[models.Role]models.Profile, error) {
	if len(roles) == 0 {
		roles = []models.Role{models.RoleKWS, models.RoleVAD, models.RoleSTT, models.RoleSpeaker, models.RoleTTS, models.RoleIntent}
	}
	required := make(map[models.Role]models.Profile, len(roles))
	for _, role := range roles {
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

func (r *configuredRuntime) commandDependencies(ctx context.Context) commandDependencies {
	modelSpeaker := runtimeSpeaker{models: r.modelSet, store: r.speakerStore}
	return commandDependencies{
		Logger:      r.logger,
		Player:      r.player,
		Synthesizer: r.modelSet,
		Transcriber: r.modelSet,
		Speaker:     modelSpeaker,
		Enroll: func(enrollCtx context.Context, id string) error {
			return enrollFromCapture(enrollCtx, id, r.capture, r.speakerStore, modelSpeaker)
		},
	}
}

func enrollFromCapture(ctx context.Context, id string, capture audio.Capture, store *speakerstore.Store, embedder speakerstore.Embedder) error {
	if capture == nil || store == nil || embedder == nil {
		return errors.New("speaker enrollment dependencies are incomplete")
	}
	captureCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	frames, err := capture.Capture(captureCtx)
	if err != nil {
		return err
	}
	const samplesPerUtterance = audio.SampleRate
	samples := make([]audio.Audio, 0, 3)
	buffer := make([]float32, 0, samplesPerUtterance)
	for len(samples) < 3 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case frame, ok := <-frames:
			if !ok {
				return errors.New("capture ended before enrollment completed")
			}
			buffer = append(buffer, frame.Samples...)
			if len(buffer) < samplesPerUtterance {
				continue
			}
			input, err := audio.NewAudio(audio.SampleRate, audio.Channels, append([]float32(nil), buffer[:samplesPerUtterance]...))
			if err != nil {
				return err
			}
			samples = append(samples, input)
			buffer = buffer[samplesPerUtterance:]
		}
	}
	cancel()
	return store.Enroll(ctx, id, samples, embedder)
}
