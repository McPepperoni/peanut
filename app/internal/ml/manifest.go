// Package ml defines local model configuration shared by runtime adapters.
package ml

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	CPUProvider   = "cpu"
	KWSBundle     = "sherpa-onnx-kws-zipformer-wenetspeech-3.3M-2024-01-01-mobile"
	VADBundle     = "silero_vad.onnx"
	STTBundle     = "sherpa-onnx-sense-voice-zh-en-ja-ko-yue-int8-2024-07-17"
	SpeakerBundle = "3dspeaker_speech_eres2net_base_sv_zh-cn_3dspeaker_16k.onnx"
	TTSBundle     = "vits-piper-en_GB-cori-medium"
)

var (
	ErrModelMissing = errors.New("model path missing")
	ErrModelInvalid = errors.New("model configuration invalid")
)

type Paths struct {
	KWS, VAD, STT, Speaker, TTS string
}

type Manifest struct {
	Paths    Paths
	Provider string
	Threads  int
}

func (m Manifest) Validate() error {
	if m.Provider != CPUProvider {
		return fmt.Errorf("%w: provider must be %q", ErrModelInvalid, CPUProvider)
	}
	if m.Threads <= 0 {
		return fmt.Errorf("%w: threads must be positive", ErrModelInvalid)
	}
	for _, model := range []struct {
		name, path string
		dir        bool
	}{
		{"KWS", m.Paths.KWS, true},
		{"VAD", m.Paths.VAD, false},
		{"STT", m.Paths.STT, true},
		{"speaker", m.Paths.Speaker, false},
		{"TTS", m.Paths.TTS, true},
	} {
		if err := validatePath(model.name, model.path, model.dir); err != nil {
			return err
		}
	}
	return nil
}

func validatePath(name, path string, directory bool) error {
	if path == "" {
		return fmt.Errorf("%w: %s", ErrModelMissing, name)
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %s at %q", ErrModelMissing, name, path)
	}
	if err != nil {
		return fmt.Errorf("%w: %s at %q", ErrModelInvalid, name, path)
	}
	if directory {
		entries, err := os.ReadDir(path)
		if !info.IsDir() || err != nil || len(entries) == 0 {
			return fmt.Errorf("%w: %s directory at %q is empty or unreadable", ErrModelInvalid, name, path)
		}
		return nil
	}
	if !info.Mode().IsRegular() || info.Size() == 0 || !strings.EqualFold(filepath.Ext(path), ".onnx") {
		return fmt.Errorf("%w: %s at %q must be a non-empty ONNX file", ErrModelInvalid, name, path)
	}
	return nil
}
