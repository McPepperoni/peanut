package main

import (
	"context"
	"fmt"
	"path/filepath"

	"peanut/assets"
	"peanut/internal/audio"
	"peanut/internal/audio/capture"
	"peanut/internal/audio/playback"
	"peanut/internal/config"
	"peanut/internal/intent"
	"peanut/internal/ml"
	"peanut/internal/ml/sherpa"
	"peanut/internal/models"
	"peanut/internal/pipeline"
	"peanut/internal/providers"
	"peanut/internal/storage/sqlite"
)

func runConfigured(ctx context.Context, cfg config.Config, db *sqlite.DB) error {
	coordinator, err := newConfiguredCoordinator(ctx, cfg, db)
	if err != nil {
		return err
	}
	return coordinator.Run(ctx)
}

func newConfiguredCoordinator(ctx context.Context, cfg config.Config, db *sqlite.DB) (*pipeline.Coordinator, error) {
	var modelStore models.Store
	if db != nil {
		modelStore = sqlite.NewModelStore(db)
	}
	modelRegistry := models.NewRegistry(cfg.Models.Root, modelStore, nil)
	snapshot, err := modelRegistry.Scan(ctx)
	if err != nil {
		return nil, fmt.Errorf("scan model roles at %s: %w", cfg.Models.Root, err)
	}
	required := make(map[models.Role]models.Profile, 6)
	for _, role := range []models.Role{models.RoleKWS, models.RoleVAD, models.RoleSTT, models.RoleSpeaker, models.RoleTTS, models.RoleIntent} {
		profile, ok := modelRegistry.Active(role)
		if !ok {
			path, detail := filepath.Join(cfg.Models.Root, string(role)), "missing"
			for _, candidate := range snapshot.Profiles {
				if candidate.Role == role {
					path = filepath.Join(cfg.Models.Root, filepath.FromSlash(candidate.Path), filepath.FromSlash(candidate.Entry))
					detail = candidate.Error
					break
				}
			}
			return nil, fmt.Errorf("model role %q at %s is invalid: %s", role, path, detail)
		}
		required[role] = profile
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
		return nil, fmt.Errorf("create wake detector: %w", err)
	}
	voice, err := sherpa.NewVAD(manifest)
	if err != nil {
		return nil, fmt.Errorf("create VAD: %w", err)
	}
	transcriber, err := sherpa.NewTranscriber(manifest)
	if err != nil {
		return nil, fmt.Errorf("create transcriber: %w", err)
	}
	synthesizer, err := sherpa.NewSynthesizer(manifest)
	if err != nil {
		return nil, fmt.Errorf("create synthesizer: %w", err)
	}
	acknowledgement, err := assets.Acknowledgement()
	if err != nil {
		return nil, fmt.Errorf("load acknowledgement: %w", err)
	}
	registry := providers.NewRegistry()
	homeAssistant := providers.NewHomeAssistantProvider(db, nil)
	if err := registry.Register(homeAssistant); err != nil {
		return nil, err
	}
	capabilities, err := registry.Snapshot(ctx)
	if err != nil {
		return nil, fmt.Errorf("discover capabilities: %w", err)
	}
	intentProfile := required[models.RoleIntent]
	parser := intent.NewModelParser(intentProfile.Runtime, modelEntry(cfg.Models.Root, intentProfile), cfg.Models.Threads, cfg.HomeAssistant.Timeout)
	pcfg := pipeline.DefaultConfig()
	pcfg.PreRollFrames = max(1, cfg.Audio.PreRollMilliseconds*audio.SampleRate/(1000*audio.FrameSamples))
	pcfg.PreRollSamples = pcfg.PreRollFrames * audio.FrameSamples
	pcfg.VADThreshold = cfg.Audio.VADThreshold
	pcfg.AcknowledgementTail = cfg.Audio.AcknowledgementTail
	pcfg.PlaybackTail = cfg.Audio.PlaybackTail
	pcfg.NoSpeechTimeout = cfg.Audio.NoSpeechTimeout
	pcfg.MaximumCommandDuration = cfg.Audio.MaximumCommandDuration
	return pipeline.NewCoordinator(pcfg, pipeline.Dependencies{
		Capture:         capture.SystemCapture{},
		Player:          playback.SystemPlayer{},
		Acknowledgement: acknowledgement,
		WakeDetector:    wake,
		VAD:             voice,
		Transcriber:     transcriber,
		IntentParser:    parser,
		Synthesizer:     synthesizer,
		Registry:        registry,
		Capabilities:    capabilities,
	})
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
