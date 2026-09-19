package sherpa

import (
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"

	"peanut/internal/audio"
	"peanut/internal/ml"
	"peanut/internal/ml/vad"
)

func TestNativeRoleConfigsUseManifestPathsCPUAndThreads(t *testing.T) {
	manifest := validManifest(t)
	manifest.Threads = 3

	wake, err := newWakeConfig(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if wake.Encoder != filepath.Join(manifest.Paths.KWS, "encoder-epoch-12-avg-2-chunk-16-left-64.int8.onnx") ||
		wake.Decoder != filepath.Join(manifest.Paths.KWS, "decoder-epoch-12-avg-2-chunk-16-left-64.onnx") ||
		wake.Joiner != filepath.Join(manifest.Paths.KWS, "joiner-epoch-12-avg-2-chunk-16-left-64.int8.onnx") ||
		wake.Tokens != filepath.Join(manifest.Paths.KWS, "tokens.txt") ||
		wake.Keywords != filepath.Join(manifest.Paths.KWS, "keywords.txt") ||
		wake.Provider != ml.CPUProvider || wake.Threads != 3 {
		t.Fatalf("wake config = %+v", wake)
	}

	voice, err := newVADConfig(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if voice.Model != manifest.Paths.VAD || voice.Provider != ml.CPUProvider || voice.Threads != 3 || voice.Threshold != defaultVADThreshold {
		t.Fatalf("VAD config = %+v", voice)
	}

	embedder, err := newSpeakerConfig(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if embedder.Model != manifest.Paths.Speaker || embedder.Provider != ml.CPUProvider || embedder.Threads != 3 {
		t.Fatalf("speaker config = %+v", embedder)
	}

	synthesizer, err := newTTSConfig(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if synthesizer.Model != filepath.Join(manifest.Paths.TTS, "en_GB-cori-medium.onnx") ||
		synthesizer.Tokens != filepath.Join(manifest.Paths.TTS, "tokens.txt") ||
		synthesizer.DataDir != filepath.Join(manifest.Paths.TTS, "espeak-ng-data") ||
		synthesizer.Provider != ml.CPUProvider || synthesizer.Threads != 3 {
		t.Fatalf("TTS config = %+v", synthesizer)
	}
}

func TestVADThresholdOption(t *testing.T) {
	manifest := validManifest(t)
	config, err := newVADConfig(manifest, WithVADThreshold(0.25))
	if err != nil {
		t.Fatal(err)
	}
	if config.Threshold != 0.25 {
		t.Fatalf("threshold = %v, want 0.25", config.Threshold)
	}
	for _, threshold := range []float32{0, -0.1, 1.1, float32(math.NaN()), float32(math.Inf(1))} {
		if _, err := newVADConfig(manifest, WithVADThreshold(threshold)); !errors.Is(err, ml.ErrModelInvalid) {
			t.Fatalf("threshold %v error = %v, want invalid model", threshold, err)
		}
	}
}

func TestKeywordResultMapping(t *testing.T) {
	if result := mapKeywordResult(""); result.Detected || result.Keyword != "" || result.Score != 0 {
		t.Fatalf("empty keyword result = %+v", result)
	}
	if result := mapKeywordResult("  peanut  "); !result.Detected || result.Keyword != "peanut" || result.Score != 0 {
		t.Fatalf("detected keyword result = %+v", result)
	}
}

func TestVADResultMapping(t *testing.T) {
	result, active := mapVADResult(false, true, false)
	if result.Endpoint != vad.SpeechStarted || result.Probability != 1 || !active {
		t.Fatalf("speech start = %+v active=%v", result, active)
	}
	result, active = mapVADResult(active, true, false)
	if result.Endpoint != vad.NoEndpoint || result.Probability != 1 || !active {
		t.Fatalf("continued speech = %+v active=%v", result, active)
	}
	result, active = mapVADResult(active, false, true)
	if result.Endpoint != vad.SpeechEnded || result.Probability != 0 || active {
		t.Fatalf("speech end = %+v active=%v", result, active)
	}
	result, active = mapVADResult(active, false, false)
	if result.Endpoint != vad.NoEndpoint || result.Probability != 0 || active {
		t.Fatalf("silence = %+v active=%v", result, active)
	}
}

func TestSpeakerEmbeddingAndUnknownIdentityContracts(t *testing.T) {
	embedding, err := copyEmbedding([]float32{0.25, -0.5})
	if err != nil {
		t.Fatal(err)
	}
	if len(embedding) != 2 || embedding[0] != 0.25 || embedding[1] != -0.5 {
		t.Fatalf("embedding = %v", embedding)
	}
	for _, invalid := range [][]float32{nil, {0, 0}, {float32(math.NaN())}, {float32(math.Inf(1))}} {
		if _, err := copyEmbedding(invalid); err == nil {
			t.Fatalf("accepted embedding %v", invalid)
		}
	}
	if result := mapSpeakerResult(nil); result.ID != "" || result.Score != 0 || result.Err != nil {
		t.Fatalf("unknown speaker result = %+v", result)
	}
	want := errors.New("embedding failed")
	if result := mapSpeakerResult(want); !errors.Is(result.Err, want) || result.ID != "" {
		t.Fatalf("failed speaker result = %+v", result)
	}
}

func TestTTSResultMappingResamplesToRuntimeAudio(t *testing.T) {
	result, err := mapTTSResult([]float32{-1, 0, 1}, 8000)
	if err != nil {
		t.Fatal(err)
	}
	if result.Audio.SampleRate != audio.SampleRate || result.Audio.Channels != audio.Channels || len(result.Audio.Samples) != 6 {
		t.Fatalf("TTS audio = %+v", result.Audio)
	}
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestTTSResultMappingRejectsInvalidNativeOutput(t *testing.T) {
	for _, test := range []struct {
		name       string
		samples    []float32
		sampleRate int
	}{
		{"empty", nil, 16000},
		{"sample rate", []float32{0}, 0},
		{"sample", []float32{2}, 16000},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := mapTTSResult(test.samples, test.sampleRate); err == nil {
				t.Fatal("accepted invalid generated audio")
			}
		})
	}
}

func validManifest(t *testing.T) ml.Manifest {
	t.Helper()
	root := t.TempDir()
	directory := func(name string) string {
		path := filepath.Join(root, name)
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		return path
	}
	file := func(name string) string {
		path := filepath.Join(root, name)
		if err := os.WriteFile(path, []byte("model"), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	manifest := ml.Manifest{Paths: ml.Paths{
		KWS: directory(ml.KWSBundle), VAD: file(ml.VADBundle), STT: directory(ml.STTBundle),
		Speaker: file(ml.SpeakerBundle), TTS: directory(ml.TTSBundle),
	}, Provider: ml.CPUProvider, Threads: 1}
	for _, path := range []string{
		filepath.Join(manifest.Paths.KWS, "encoder-epoch-12-avg-2-chunk-16-left-64.int8.onnx"),
		filepath.Join(manifest.Paths.KWS, "decoder-epoch-12-avg-2-chunk-16-left-64.onnx"),
		filepath.Join(manifest.Paths.KWS, "joiner-epoch-12-avg-2-chunk-16-left-64.int8.onnx"),
		filepath.Join(manifest.Paths.KWS, "tokens.txt"),
		filepath.Join(manifest.Paths.KWS, "keywords.txt"),
		filepath.Join(manifest.Paths.STT, "model.int8.onnx"),
		filepath.Join(manifest.Paths.STT, "tokens.txt"),
		filepath.Join(manifest.Paths.TTS, "en_GB-cori-medium.onnx"),
		filepath.Join(manifest.Paths.TTS, "tokens.txt"),
	} {
		if err := os.WriteFile(path, []byte("model"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(manifest.Paths.TTS, "espeak-ng-data"), 0o755); err != nil {
		t.Fatal(err)
	}
	return manifest
}

func writeSTTFiles(t *testing.T, directory string) {
	t.Helper()
	for _, name := range []string{"model.int8.onnx", "tokens.txt"} {
		if err := os.WriteFile(filepath.Join(directory, name), []byte("model"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}
