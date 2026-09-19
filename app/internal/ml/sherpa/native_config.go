package sherpa

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"

	"peanut/internal/audio"
	"peanut/internal/ml"
	"peanut/internal/ml/kws"
	"peanut/internal/ml/speaker"
	"peanut/internal/ml/tts"
	"peanut/internal/ml/vad"
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

type wakeConfig struct {
	Encoder, Decoder, Joiner, Tokens, Keywords, Provider string
	Threads                                              int
}

type vadConfig struct {
	Model, Provider string
	Threads         int
	Threshold       float32
}

type speakerConfig struct {
	Model, Provider string
	Threads         int
}

type ttsConfig struct {
	Model, Tokens, DataDir, Provider string
	Threads                          int
}

const defaultVADThreshold float32 = 0.5

// VADOption customizes native Silero VAD construction without changing existing callers.
type VADOption interface{ apply(*vadConfig) error }

type vadOptionFunc func(*vadConfig) error

func (f vadOptionFunc) apply(config *vadConfig) error { return f(config) }

// WithVADThreshold overrides default 0.5 Silero speech threshold.
func WithVADThreshold(threshold float32) VADOption {
	return vadOptionFunc(func(config *vadConfig) error {
		if threshold <= 0 || threshold > 1 || math.IsNaN(float64(threshold)) || math.IsInf(float64(threshold), 0) {
			return fmt.Errorf("%w: VAD threshold must be finite and within (0, 1]", ml.ErrModelInvalid)
		}
		config.Threshold = threshold
		return nil
	})
}

func newWakeConfig(manifest ml.Manifest) (wakeConfig, error) {
	if err := manifest.ValidateRole("kws"); err != nil {
		return wakeConfig{}, err
	}
	return wakeConfig{
		Encoder:  filepath.Join(manifest.Paths.KWS, "encoder-epoch-12-avg-2-chunk-16-left-64.int8.onnx"),
		Decoder:  filepath.Join(manifest.Paths.KWS, "decoder-epoch-12-avg-2-chunk-16-left-64.onnx"),
		Joiner:   filepath.Join(manifest.Paths.KWS, "joiner-epoch-12-avg-2-chunk-16-left-64.int8.onnx"),
		Tokens:   filepath.Join(manifest.Paths.KWS, "tokens.txt"),
		Keywords: filepath.Join(manifest.Paths.KWS, "keywords.txt"),
		Provider: ml.CPUProvider,
		Threads:  manifest.Threads,
	}, nil
}

func newVADConfig(manifest ml.Manifest, options ...VADOption) (vadConfig, error) {
	if err := manifest.ValidateRole("vad"); err != nil {
		return vadConfig{}, err
	}
	config := vadConfig{Model: manifest.Paths.VAD, Provider: ml.CPUProvider, Threads: manifest.Threads, Threshold: defaultVADThreshold}
	for _, option := range options {
		if option == nil {
			return vadConfig{}, fmt.Errorf("%w: nil VAD option", ml.ErrModelInvalid)
		}
		if err := option.apply(&config); err != nil {
			return vadConfig{}, err
		}
	}
	return config, nil
}

func newSpeakerConfig(manifest ml.Manifest) (speakerConfig, error) {
	if err := manifest.ValidateRole("speaker"); err != nil {
		return speakerConfig{}, err
	}
	return speakerConfig{Model: manifest.Paths.Speaker, Provider: ml.CPUProvider, Threads: manifest.Threads}, nil
}

func newTTSConfig(manifest ml.Manifest) (ttsConfig, error) {
	if err := manifest.ValidateRole("tts"); err != nil {
		return ttsConfig{}, err
	}
	return ttsConfig{
		Model:    filepath.Join(manifest.Paths.TTS, "en_GB-cori-medium.onnx"),
		Tokens:   filepath.Join(manifest.Paths.TTS, "tokens.txt"),
		DataDir:  filepath.Join(manifest.Paths.TTS, "espeak-ng-data"),
		Provider: ml.CPUProvider,
		Threads:  manifest.Threads,
	}, nil
}

func mapKeywordResult(keyword string) kws.Result {
	keyword = strings.TrimSpace(keyword)
	return kws.Result{Detected: keyword != "", Keyword: keyword}
}

func mapVADResult(wasDetected, detected, queued bool) (vad.Result, bool) {
	probability := float32(0)
	if detected {
		probability = 1
	}
	if queued {
		return vad.Result{Endpoint: vad.SpeechEnded, Probability: probability}, false
	}
	if detected && !wasDetected {
		return vad.Result{Endpoint: vad.SpeechStarted, Probability: probability}, true
	}
	return vad.Result{Probability: probability}, detected
}

func copyEmbedding(values []float32) ([]float32, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("%w: empty speaker embedding", ErrNativeInference)
	}
	var magnitude float64
	for _, value := range values {
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			return nil, fmt.Errorf("%w: non-finite speaker embedding", ErrNativeInference)
		}
		magnitude += float64(value) * float64(value)
	}
	if magnitude == 0 || math.IsInf(magnitude, 0) {
		return nil, fmt.Errorf("%w: zero speaker embedding", ErrNativeInference)
	}
	return append([]float32(nil), values...), nil
}

func mapSpeakerResult(err error) speaker.Result { return speaker.Result{Err: err} }

func mapTTSResult(samples []float32, sampleRate int) (tts.Result, error) {
	if sampleRate <= 0 || len(samples) == 0 {
		return tts.Result{}, fmt.Errorf("%w: invalid generated audio metadata", ErrNativeInference)
	}
	for _, sample := range samples {
		if sample < -1 || sample > 1 || math.IsNaN(float64(sample)) || math.IsInf(float64(sample), 0) {
			return tts.Result{}, fmt.Errorf("%w: generated sample outside [-1, 1]", ErrNativeInference)
		}
	}
	if sampleRate != audio.SampleRate {
		samples = resampleLinear(samples, sampleRate, audio.SampleRate)
	}
	output, err := audio.NewAudio(audio.SampleRate, audio.Channels, samples)
	if err != nil {
		return tts.Result{}, fmt.Errorf("%w: %v", ErrNativeInference, err)
	}
	return tts.Result{Audio: output}, nil
}

func resampleLinear(input []float32, sourceRate, targetRate int) []float32 {
	outputLength := int(math.Round(float64(len(input)) * float64(targetRate) / float64(sourceRate)))
	if outputLength < 1 {
		outputLength = 1
	}
	output := make([]float32, outputLength)
	ratio := float64(sourceRate) / float64(targetRate)
	for index := range output {
		position := float64(index) * ratio
		left := min(int(position), len(input)-1)
		right := min(left+1, len(input)-1)
		fraction := float32(position - float64(left))
		output[index] = input[left] + (input[right]-input[left])*fraction
	}
	return output
}

func newNativeConfig(manifest ml.Manifest) (nativeConfig, error) {
	if err := manifest.ValidateRole("stt"); err != nil {
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
