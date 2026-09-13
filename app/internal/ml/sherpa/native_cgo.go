//go:build cgo && sherpa

package sherpa

/*
#cgo LDFLAGS: -lsherpa-onnx-c-api
#include <stdlib.h>
#include <sherpa-onnx/c-api/c-api.h>
*/
import "C"

import (
	"context"
	"fmt"
	"runtime"
	"unsafe"

	"peanut/internal/audio"
	"peanut/internal/ml"
	"peanut/internal/ml/kws"
	"peanut/internal/ml/speaker"
	"peanut/internal/ml/stt"
	"peanut/internal/ml/tts"
	"peanut/internal/ml/vad"
)

type WakeDetector struct{}
type VoiceActivityDetector struct{}
type Transcriber struct{ manifest ml.Manifest }
type SpeakerIdentifier struct{}
type Synthesizer struct{}

func nativeUnavailable(component string) error {
	return fmt.Errorf("%w: %s C API adapter is not implemented", ErrUnavailable, component)
}

func NewWakeDetector(manifest ml.Manifest) (*WakeDetector, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return &WakeDetector{}, nil
}

func NewVAD(manifest ml.Manifest) (*VoiceActivityDetector, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return &VoiceActivityDetector{}, nil
}

func NewTranscriber(manifest ml.Manifest) (*Transcriber, error) {
	if _, err := newNativeConfig(manifest); err != nil {
		return nil, err
	}
	return &Transcriber{manifest: manifest}, nil
}

func NewSpeakerIdentifier(manifest ml.Manifest) (*SpeakerIdentifier, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return &SpeakerIdentifier{}, nil
}

func NewSynthesizer(manifest ml.Manifest) (*Synthesizer, error) {
	if err := manifest.Validate(); err != nil {
		return nil, err
	}
	return &Synthesizer{}, nil
}

func (*WakeDetector) Detect(_ context.Context, frame audio.Frame) (kws.Result, error) {
	if err := frame.Validate(); err != nil {
		return kws.Result{}, err
	}
	return kws.Result{}, nativeUnavailable("KWS")
}

func (*WakeDetector) Reset() error { return nativeUnavailable("KWS") }

func (*VoiceActivityDetector) Detect(_ context.Context, frame audio.Frame) (vad.Result, error) {
	if err := frame.Validate(); err != nil {
		return vad.Result{}, err
	}
	return vad.Result{}, nativeUnavailable("VAD")
}

func (*VoiceActivityDetector) Reset() error { return nativeUnavailable("VAD") }

func (t *Transcriber) Transcribe(_ context.Context, input audio.Audio) (stt.Result, error) {
	text, err := Transcribe(t.manifest, input)
	return stt.Result{Text: text}, err
}

func (*SpeakerIdentifier) Identify(_ context.Context, input audio.Audio) speaker.Result {
	if err := input.Validate(); err != nil {
		return speaker.Result{Err: err}
	}
	return speaker.Result{Err: nativeUnavailable("speaker identification")}
}

func (*Synthesizer) Synthesize(_ context.Context, _ string) (tts.Result, error) {
	return tts.Result{}, nativeUnavailable("TTS")
}

// Open validates configuration and proves the native SenseVoice model loads.
func Open(manifest ml.Manifest) error {
	config, err := newNativeConfig(manifest)
	if err != nil {
		return err
	}
	recognizer, err := openRecognizer(config)
	if err != nil {
		return err
	}
	C.SherpaOnnxDestroyOfflineRecognizer(recognizer)
	return nil
}

// Transcribe is the narrow native inference boundary used by the smoke test.
func Transcribe(manifest ml.Manifest, input audio.Audio) (string, error) {
	config, err := newNativeConfig(manifest)
	if err != nil {
		return "", err
	}
	if err := input.Validate(); err != nil {
		return "", err
	}
	recognizer, err := openRecognizer(config)
	if err != nil {
		return "", err
	}
	defer C.SherpaOnnxDestroyOfflineRecognizer(recognizer)

	stream := C.SherpaOnnxCreateOfflineStream(recognizer)
	if stream == nil {
		return "", fmt.Errorf("%w: create offline STT stream", ErrNativeInference)
	}
	defer C.SherpaOnnxDestroyOfflineStream(stream)

	C.SherpaOnnxAcceptWaveformOffline(
		stream,
		C.int32_t(audio.SampleRate),
		(*C.float)(unsafe.Pointer(&input.Samples[0])),
		C.int32_t(len(input.Samples)),
	)
	C.SherpaOnnxDecodeOfflineStream(recognizer, stream)
	result := C.SherpaOnnxGetOfflineStreamResult(stream)
	runtime.KeepAlive(input.Samples)
	if result == nil || result.text == nil {
		return "", fmt.Errorf("%w: decode offline STT", ErrNativeInference)
	}
	defer C.SherpaOnnxDestroyOfflineRecognizerResult(result)
	return C.GoString(result.text), nil
}

func openRecognizer(config nativeConfig) (*C.SherpaOnnxOfflineRecognizer, error) {
	if config.Provider != ml.CPUProvider || config.Threads <= 0 {
		return nil, fmt.Errorf("%w: provider must be %q and threads positive", ml.ErrModelInvalid, ml.CPUProvider)
	}
	model := C.CString(config.Model)
	tokens := C.CString(config.Tokens)
	provider := C.CString(ml.CPUProvider)
	language := C.CString("auto")
	decodingMethod := C.CString("greedy_search")
	defer C.free(unsafe.Pointer(model))
	defer C.free(unsafe.Pointer(tokens))
	defer C.free(unsafe.Pointer(provider))
	defer C.free(unsafe.Pointer(language))
	defer C.free(unsafe.Pointer(decodingMethod))

	var native C.SherpaOnnxOfflineRecognizerConfig
	native.feat_config.sample_rate = C.int32_t(audio.SampleRate)
	native.feat_config.feature_dim = 80
	native.model_config.sense_voice.model = model
	native.model_config.sense_voice.language = language
	native.model_config.sense_voice.use_itn = 1
	native.model_config.tokens = tokens
	native.model_config.provider = provider
	native.model_config.num_threads = C.int32_t(config.Threads)
	native.decoding_method = decodingMethod

	recognizer := C.SherpaOnnxCreateOfflineRecognizer(&native)
	if recognizer == nil {
		return nil, fmt.Errorf("%w: SenseVoice model %q", ErrNativeLoad, config.Model)
	}
	return recognizer, nil
}
