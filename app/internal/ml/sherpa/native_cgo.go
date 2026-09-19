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
	"strings"
	"sync"
	"unsafe"

	"peanut/internal/audio"
	"peanut/internal/ml"
	"peanut/internal/ml/kws"
	"peanut/internal/ml/speaker"
	"peanut/internal/ml/stt"
	"peanut/internal/ml/tts"
	"peanut/internal/ml/vad"
)

type WakeDetector struct {
	mu      sync.Mutex
	spotter *C.SherpaOnnxKeywordSpotter
	stream  *C.SherpaOnnxOnlineStream
}

type VoiceActivityDetector struct {
	mu       sync.Mutex
	detector *C.SherpaOnnxVoiceActivityDetector
	pending  []float32
	inSpeech bool
}

type Transcriber struct{ manifest ml.Manifest }

type SpeakerIdentifier struct {
	mu        sync.Mutex
	extractor *C.SherpaOnnxSpeakerEmbeddingExtractor
	dimension int
}

type Synthesizer struct {
	mu     sync.Mutex
	native *C.SherpaOnnxOfflineTts
}

func NewWakeDetector(manifest ml.Manifest) (*WakeDetector, error) {
	config, err := newWakeConfig(manifest)
	if err != nil {
		return nil, err
	}
	spotter, stream, err := openKeywordSpotter(config)
	if err != nil {
		return nil, err
	}
	detector := &WakeDetector{spotter: spotter, stream: stream}
	runtime.SetFinalizer(detector, (*WakeDetector).finalize)
	return detector, nil
}

func NewVAD(manifest ml.Manifest, options ...VADOption) (*VoiceActivityDetector, error) {
	config, err := newVADConfig(manifest, options...)
	if err != nil {
		return nil, err
	}
	detector, err := openVoiceActivityDetector(config)
	if err != nil {
		return nil, err
	}
	voice := &VoiceActivityDetector{detector: detector, pending: make([]float32, 0, 512)}
	runtime.SetFinalizer(voice, (*VoiceActivityDetector).finalize)
	return voice, nil
}

func NewTranscriber(manifest ml.Manifest) (*Transcriber, error) {
	if _, err := newNativeConfig(manifest); err != nil {
		return nil, err
	}
	return &Transcriber{manifest: manifest}, nil
}

func NewSpeakerIdentifier(manifest ml.Manifest) (*SpeakerIdentifier, error) {
	config, err := newSpeakerConfig(manifest)
	if err != nil {
		return nil, err
	}
	extractor, dimension, err := openSpeakerEmbeddingExtractor(config)
	if err != nil {
		return nil, err
	}
	identifier := &SpeakerIdentifier{extractor: extractor, dimension: dimension}
	runtime.SetFinalizer(identifier, (*SpeakerIdentifier).finalize)
	return identifier, nil
}

func NewSynthesizer(manifest ml.Manifest) (*Synthesizer, error) {
	config, err := newTTSConfig(manifest)
	if err != nil {
		return nil, err
	}
	native, err := openSynthesizer(config)
	if err != nil {
		return nil, err
	}
	synthesizer := &Synthesizer{native: native}
	runtime.SetFinalizer(synthesizer, (*Synthesizer).finalize)
	return synthesizer, nil
}

func (w *WakeDetector) Detect(ctx context.Context, frame audio.Frame) (kws.Result, error) {
	if err := frame.Validate(); err != nil {
		return kws.Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return kws.Result{}, err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.spotter == nil || w.stream == nil {
		return kws.Result{}, fmt.Errorf("%w: KWS detector is closed", ErrNativeInference)
	}
	C.SherpaOnnxOnlineStreamAcceptWaveform(
		w.stream,
		C.int32_t(audio.SampleRate),
		(*C.float)(unsafe.Pointer(&frame.Samples[0])),
		C.int32_t(len(frame.Samples)),
	)
	runtime.KeepAlive(frame.Samples)
	for C.SherpaOnnxIsKeywordStreamReady(w.spotter, w.stream) != 0 {
		if err := ctx.Err(); err != nil {
			return kws.Result{}, err
		}
		C.SherpaOnnxDecodeKeywordStream(w.spotter, w.stream)
		result := C.SherpaOnnxGetKeywordResult(w.spotter, w.stream)
		if result == nil {
			return kws.Result{}, fmt.Errorf("%w: read KWS result", ErrNativeInference)
		}
		keyword := ""
		if result.keyword != nil {
			keyword = C.GoString(result.keyword)
		}
		C.SherpaOnnxDestroyKeywordResult(result)
		mapped := mapKeywordResult(keyword)
		if mapped.Detected {
			C.SherpaOnnxResetKeywordStream(w.spotter, w.stream)
			return mapped, nil
		}
	}
	return kws.Result{}, ctx.Err()
}

func (w *WakeDetector) Reset() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.spotter == nil || w.stream == nil {
		return fmt.Errorf("%w: KWS detector is closed", ErrNativeInference)
	}
	C.SherpaOnnxResetKeywordStream(w.spotter, w.stream)
	return nil
}

func (v *VoiceActivityDetector) Detect(ctx context.Context, frame audio.Frame) (vad.Result, error) {
	if err := frame.Validate(); err != nil {
		return vad.Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return vad.Result{}, err
	}
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.detector == nil {
		return vad.Result{}, fmt.Errorf("%w: VAD is closed", ErrNativeInference)
	}
	v.pending = append(v.pending, frame.Samples...)
	if len(v.pending) < 512 {
		result, active := mapVADResult(v.inSpeech, v.inSpeech, false)
		v.inSpeech = active
		return result, nil
	}
	C.SherpaOnnxVoiceActivityDetectorAcceptWaveform(
		v.detector,
		(*C.float)(unsafe.Pointer(&v.pending[0])),
		512,
	)
	runtime.KeepAlive(v.pending)
	v.pending = append(v.pending[:0], v.pending[512:]...)
	detected := C.SherpaOnnxVoiceActivityDetectorDetected(v.detector) != 0
	queued := C.SherpaOnnxVoiceActivityDetectorEmpty(v.detector) == 0
	for C.SherpaOnnxVoiceActivityDetectorEmpty(v.detector) == 0 {
		C.SherpaOnnxVoiceActivityDetectorPop(v.detector)
	}
	result, active := mapVADResult(v.inSpeech, detected, queued)
	v.inSpeech = active
	if err := ctx.Err(); err != nil {
		return vad.Result{}, err
	}
	return result, nil
}

func (v *VoiceActivityDetector) Reset() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.detector == nil {
		return fmt.Errorf("%w: VAD is closed", ErrNativeInference)
	}
	C.SherpaOnnxVoiceActivityDetectorReset(v.detector)
	v.pending = v.pending[:0]
	v.inSpeech = false
	return nil
}

func (t *Transcriber) Transcribe(ctx context.Context, input audio.Audio) (stt.Result, error) {
	if err := ctx.Err(); err != nil {
		return stt.Result{}, err
	}
	text, err := Transcribe(t.manifest, input)
	if err == nil {
		err = ctx.Err()
	}
	return stt.Result{Text: text}, err
}

func (s *SpeakerIdentifier) Identify(ctx context.Context, input audio.Audio) speaker.Result {
	_, err := s.Embed(ctx, input)
	return mapSpeakerResult(err)
}

func (s *SpeakerIdentifier) Embed(ctx context.Context, input audio.Audio) ([]float32, error) {
	if err := input.Validate(); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.extractor == nil || s.dimension <= 0 {
		return nil, fmt.Errorf("%w: speaker extractor is closed", ErrNativeInference)
	}
	stream := C.SherpaOnnxSpeakerEmbeddingExtractorCreateStream(s.extractor)
	if stream == nil {
		return nil, fmt.Errorf("%w: create speaker stream", ErrNativeInference)
	}
	defer C.SherpaOnnxDestroyOnlineStream(stream)
	C.SherpaOnnxOnlineStreamAcceptWaveform(
		stream,
		C.int32_t(audio.SampleRate),
		(*C.float)(unsafe.Pointer(&input.Samples[0])),
		C.int32_t(len(input.Samples)),
	)
	C.SherpaOnnxOnlineStreamInputFinished(stream)
	runtime.KeepAlive(input.Samples)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if C.SherpaOnnxSpeakerEmbeddingExtractorIsReady(s.extractor, stream) == 0 {
		return nil, fmt.Errorf("%w: speaker audio is too short", ErrNativeInference)
	}
	native := C.SherpaOnnxSpeakerEmbeddingExtractorComputeEmbedding(s.extractor, stream)
	if native == nil {
		return nil, fmt.Errorf("%w: compute speaker embedding", ErrNativeInference)
	}
	defer C.SherpaOnnxSpeakerEmbeddingExtractorDestroyEmbedding(native)
	embedding, err := copyEmbedding(unsafe.Slice((*float32)(unsafe.Pointer(native)), s.dimension))
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return embedding, nil
}

func (s *Synthesizer) Synthesize(ctx context.Context, text string) (tts.Result, error) {
	if strings.TrimSpace(text) == "" {
		return tts.Result{}, fmt.Errorf("%w: TTS text is empty", ErrNativeInference)
	}
	if err := ctx.Err(); err != nil {
		return tts.Result{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.native == nil {
		return tts.Result{}, fmt.Errorf("%w: TTS synthesizer is closed", ErrNativeInference)
	}
	nativeText := C.CString(text)
	defer C.free(unsafe.Pointer(nativeText))
	var generation C.SherpaOnnxGenerationConfig
	generation.speed = 1
	generation.sid = 0
	generation.silence_scale = 0.2
	generated := C.SherpaOnnxOfflineTtsGenerateWithConfig(s.native, nativeText, &generation, nil, nil)
	if generated == nil {
		return tts.Result{}, fmt.Errorf("%w: generate TTS audio", ErrNativeInference)
	}
	defer C.SherpaOnnxDestroyOfflineTtsGeneratedAudio(generated)
	if err := ctx.Err(); err != nil {
		return tts.Result{}, err
	}
	if generated.samples == nil || generated.n <= 0 || generated.sample_rate <= 0 {
		return tts.Result{}, fmt.Errorf("%w: invalid generated TTS audio", ErrNativeInference)
	}
	samples := append([]float32(nil), unsafe.Slice((*float32)(unsafe.Pointer(generated.samples)), int(generated.n))...)
	result, err := mapTTSResult(samples, int(generated.sample_rate))
	if err != nil {
		return tts.Result{}, err
	}
	if err := ctx.Err(); err != nil {
		return tts.Result{}, err
	}
	return result, nil
}

func (w *WakeDetector) Close() error {
	runtime.SetFinalizer(w, nil)
	return w.close()
}

func (w *WakeDetector) close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stream != nil {
		C.SherpaOnnxDestroyOnlineStream(w.stream)
		w.stream = nil
	}
	if w.spotter != nil {
		C.SherpaOnnxDestroyKeywordSpotter(w.spotter)
		w.spotter = nil
	}
	return nil
}

func (w *WakeDetector) finalize() { _ = w.close() }

func (v *VoiceActivityDetector) Close() error {
	runtime.SetFinalizer(v, nil)
	return v.close()
}

func (v *VoiceActivityDetector) close() error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.detector != nil {
		C.SherpaOnnxDestroyVoiceActivityDetector(v.detector)
		v.detector = nil
	}
	v.pending = nil
	v.inSpeech = false
	return nil
}

func (v *VoiceActivityDetector) finalize() { _ = v.close() }

func (s *SpeakerIdentifier) Close() error {
	runtime.SetFinalizer(s, nil)
	return s.close()
}

func (s *SpeakerIdentifier) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.extractor != nil {
		C.SherpaOnnxDestroySpeakerEmbeddingExtractor(s.extractor)
		s.extractor = nil
	}
	s.dimension = 0
	return nil
}

func (s *SpeakerIdentifier) finalize() { _ = s.close() }

func (s *Synthesizer) Close() error {
	runtime.SetFinalizer(s, nil)
	return s.close()
}

func (s *Synthesizer) close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.native != nil {
		C.SherpaOnnxDestroyOfflineTts(s.native)
		s.native = nil
	}
	return nil
}

func (s *Synthesizer) finalize() { _ = s.close() }

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

func openKeywordSpotter(config wakeConfig) (*C.SherpaOnnxKeywordSpotter, *C.SherpaOnnxOnlineStream, error) {
	encoder := C.CString(config.Encoder)
	decoder := C.CString(config.Decoder)
	joiner := C.CString(config.Joiner)
	tokens := C.CString(config.Tokens)
	keywords := C.CString(config.Keywords)
	provider := C.CString(config.Provider)
	defer C.free(unsafe.Pointer(encoder))
	defer C.free(unsafe.Pointer(decoder))
	defer C.free(unsafe.Pointer(joiner))
	defer C.free(unsafe.Pointer(tokens))
	defer C.free(unsafe.Pointer(keywords))
	defer C.free(unsafe.Pointer(provider))

	var native C.SherpaOnnxKeywordSpotterConfig
	native.feat_config.sample_rate = C.int32_t(audio.SampleRate)
	native.feat_config.feature_dim = 80
	native.model_config.transducer.encoder = encoder
	native.model_config.transducer.decoder = decoder
	native.model_config.transducer.joiner = joiner
	native.model_config.tokens = tokens
	native.model_config.provider = provider
	native.model_config.num_threads = C.int32_t(config.Threads)
	native.max_active_paths = 4
	native.num_trailing_blanks = 1
	native.keywords_score = 3
	native.keywords_threshold = 0.1
	native.keywords_file = keywords

	spotter := C.SherpaOnnxCreateKeywordSpotter(&native)
	if spotter == nil {
		return nil, nil, fmt.Errorf("%w: KWS model %q", ErrNativeLoad, config.Encoder)
	}
	stream := C.SherpaOnnxCreateKeywordStream(spotter)
	if stream == nil {
		C.SherpaOnnxDestroyKeywordSpotter(spotter)
		return nil, nil, fmt.Errorf("%w: create KWS stream", ErrNativeLoad)
	}
	return spotter, stream, nil
}

func openVoiceActivityDetector(config vadConfig) (*C.SherpaOnnxVoiceActivityDetector, error) {
	model := C.CString(config.Model)
	provider := C.CString(config.Provider)
	defer C.free(unsafe.Pointer(model))
	defer C.free(unsafe.Pointer(provider))

	var native C.SherpaOnnxVadModelConfig
	native.silero_vad.model = model
	native.silero_vad.threshold = C.float(config.Threshold)
	native.silero_vad.min_silence_duration = 0.5
	native.silero_vad.min_speech_duration = 0.25
	native.silero_vad.max_speech_duration = 30
	native.silero_vad.window_size = 512
	native.sample_rate = C.int32_t(audio.SampleRate)
	native.num_threads = C.int32_t(config.Threads)
	native.provider = provider

	detector := C.SherpaOnnxCreateVoiceActivityDetector(&native, 30)
	if detector == nil {
		return nil, fmt.Errorf("%w: Silero VAD model %q", ErrNativeLoad, config.Model)
	}
	return detector, nil
}

func openSpeakerEmbeddingExtractor(config speakerConfig) (*C.SherpaOnnxSpeakerEmbeddingExtractor, int, error) {
	model := C.CString(config.Model)
	provider := C.CString(config.Provider)
	defer C.free(unsafe.Pointer(model))
	defer C.free(unsafe.Pointer(provider))

	var native C.SherpaOnnxSpeakerEmbeddingExtractorConfig
	native.model = model
	native.num_threads = C.int32_t(config.Threads)
	native.provider = provider
	extractor := C.SherpaOnnxCreateSpeakerEmbeddingExtractor(&native)
	if extractor == nil {
		return nil, 0, fmt.Errorf("%w: speaker model %q", ErrNativeLoad, config.Model)
	}
	dimension := int(C.SherpaOnnxSpeakerEmbeddingExtractorDim(extractor))
	if dimension <= 0 {
		C.SherpaOnnxDestroySpeakerEmbeddingExtractor(extractor)
		return nil, 0, fmt.Errorf("%w: invalid speaker embedding dimension", ErrNativeLoad)
	}
	return extractor, dimension, nil
}

func openSynthesizer(config ttsConfig) (*C.SherpaOnnxOfflineTts, error) {
	model := C.CString(config.Model)
	tokens := C.CString(config.Tokens)
	dataDir := C.CString(config.DataDir)
	provider := C.CString(config.Provider)
	defer C.free(unsafe.Pointer(model))
	defer C.free(unsafe.Pointer(tokens))
	defer C.free(unsafe.Pointer(dataDir))
	defer C.free(unsafe.Pointer(provider))

	var native C.SherpaOnnxOfflineTtsConfig
	native.model.vits.model = model
	native.model.vits.tokens = tokens
	native.model.vits.data_dir = dataDir
	native.model.vits.noise_scale = 0.667
	native.model.vits.noise_scale_w = 0.8
	native.model.vits.length_scale = 1
	native.model.num_threads = C.int32_t(config.Threads)
	native.model.provider = provider
	synthesizer := C.SherpaOnnxCreateOfflineTts(&native)
	if synthesizer == nil {
		return nil, fmt.Errorf("%w: VITS model %q", ErrNativeLoad, config.Model)
	}
	return synthesizer, nil
}
