//go:build cgo && sherpa

package sherpa

/*
#cgo LDFLAGS: -lsherpa-onnx-c-api
#include <stdlib.h>
#include <sherpa-onnx/c-api/c-api.h>
*/
import "C"

import (
	"fmt"
	"runtime"
	"unsafe"

	"peanut/internal/audio"
	"peanut/internal/ml"
)

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
