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
	for _, role := range []string{"kws", "vad", "stt", "speaker", "tts"} {
		if err := m.ValidateRole(role); err != nil {
			return err
		}
	}
	return nil
}

func (m Manifest) ValidateRole(role string) error {
	if m.Provider != CPUProvider {
		return fmt.Errorf("%w: provider must be %q", ErrModelInvalid, CPUProvider)
	}
	if m.Threads <= 0 {
		return fmt.Errorf("%w: threads must be positive", ErrModelInvalid)
	}
	switch role {
	case "kws":
		if err := validatePath("KWS", m.Paths.KWS, true); err != nil {
			return err
		}
		for _, file := range []string{"encoder-epoch-12-avg-2-chunk-16-left-64.int8.onnx", "decoder-epoch-12-avg-2-chunk-16-left-64.onnx", "joiner-epoch-12-avg-2-chunk-16-left-64.int8.onnx", "tokens.txt", "keywords.txt"} {
			if err := validateRequiredFile("KWS "+file, filepath.Join(m.Paths.KWS, file)); err != nil {
				return err
			}
		}
	case "vad":
		return validatePath("VAD", m.Paths.VAD, false)
	case "stt":
		if err := validatePath("STT", m.Paths.STT, true); err != nil {
			return err
		}
		for _, file := range []string{"model.int8.onnx", "tokens.txt"} {
			if err := validateRequiredFile("STT "+file, filepath.Join(m.Paths.STT, file)); err != nil {
				return err
			}
		}
	case "speaker":
		return validatePath("speaker", m.Paths.Speaker, false)
	case "tts":
		if err := validatePath("TTS", m.Paths.TTS, true); err != nil {
			return err
		}
		for _, file := range []string{"en_GB-cori-medium.onnx", "tokens.txt"} {
			if err := validateRequiredFile("TTS "+file, filepath.Join(m.Paths.TTS, file)); err != nil {
				return err
			}
		}
		info, err := os.Stat(filepath.Join(m.Paths.TTS, "espeak-ng-data"))
		if err != nil || !info.IsDir() {
			return fmt.Errorf("%w: TTS espeak-ng-data directory", ErrModelInvalid)
		}
	default:
		return fmt.Errorf("%w: unknown role %q", ErrModelInvalid, role)
	}
	return nil
}

func validateRequiredFile(name, path string) error {
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("%w: %s at %q must be a non-empty file", ErrModelInvalid, name, path)
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
