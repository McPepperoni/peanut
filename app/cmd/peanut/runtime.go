package main

import (
	"context"
	"fmt"

	"peanut/assets"
	"peanut/internal/audio"
	"peanut/internal/audio/capture"
	"peanut/internal/audio/playback"
	"peanut/internal/config"
	"peanut/internal/intent"
	"peanut/internal/ml"
	"peanut/internal/ml/sherpa"
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
	manifest := ml.Manifest{
		Paths: ml.Paths{
			KWS:     cfg.Models.WakeWordPath,
			VAD:     cfg.Models.VADPath,
			STT:     cfg.Models.STTPath,
			Speaker: cfg.Models.SpeakerPath,
			TTS:     cfg.Models.TTSPath,
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
	parser := intent.NewQwenParser(cfg.Models.LlamaPath, cfg.Models.IntentPath, cfg.Models.Threads, cfg.HomeAssistant.Timeout)
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

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}
