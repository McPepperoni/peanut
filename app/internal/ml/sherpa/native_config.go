package sherpa

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"peanut/internal/ml"
)

var (
	ErrUnavailable     = errors.New("sherpa native runtime unavailable")
	ErrNativeLoad      = errors.New("sherpa native model load failed")
	ErrNativeInference = errors.New("sherpa native inference failed")
)

type nativeConfig struct {
	Model, Tokens, Provider string
	Threads                 int
}

func newNativeConfig(manifest ml.Manifest) (nativeConfig, error) {
	if err := manifest.Validate(); err != nil {
		return nativeConfig{}, err
	}
	model := filepath.Join(manifest.Paths.STT, "model.int8.onnx")
	tokens := filepath.Join(manifest.Paths.STT, "tokens.txt")
	for _, path := range []string{model, tokens} {
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return nativeConfig{}, fmt.Errorf("%w: required STT file at %q", ml.ErrModelInvalid, path)
		}
	}
	return nativeConfig{Model: model, Tokens: tokens, Provider: ml.CPUProvider, Threads: manifest.Threads}, nil
}
