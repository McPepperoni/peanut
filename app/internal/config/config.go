package config

import (
	"errors"
	"time"
)

const DefaultDatabasePath = "data/peanut.db"

type Config struct {
	DatabasePath  string
	Audio         Audio
	Models        Models
	HomeAssistant HomeAssistant
	API           API
}

type Audio struct {
	InputDevice            string
	OutputDevice           string
	SampleRate             int
	Channels               int
	FrameSamples           int
	PreRollMilliseconds    int
	AcknowledgementTail    time.Duration
	PlaybackTail           time.Duration
	VADThreshold           float32
	NoSpeechTimeout        time.Duration
	MaximumCommandDuration time.Duration
	DebugAudio             bool
	DebugAudioPath         string
}

type Models struct {
	WakeWordPath string
	VADPath      string
	STTPath      string
	SpeakerPath  string
	TTSPath      string
	IntentPath   string
	LlamaPath    string
	Threads      int
	CPUOnly      bool
}

type HomeAssistant struct {
	URL     string
	Token   string
	Timeout time.Duration
}

type API struct {
	Address      string
	AllowLAN     bool
	PairingToken string
}

func Load(path string) (Config, error) {
	if path == "" {
		return Config{}, errors.New("database path is required")
	}
	return Config{
		DatabasePath: path,
		Audio: Audio{
			SampleRate:             16000,
			Channels:               1,
			FrameSamples:           320,
			PreRollMilliseconds:    300,
			AcknowledgementTail:    300 * time.Millisecond,
			PlaybackTail:           300 * time.Millisecond,
			VADThreshold:           0.5,
			NoSpeechTimeout:        5 * time.Second,
			MaximumCommandDuration: 30 * time.Second,
			DebugAudioPath:         "data/debug-audio",
		},
		Models: Models{
			WakeWordPath: "local-model/sherpa-onnx-kws-zipformer-wenetspeech-3.3M-2024-01-01-mobile",
			VADPath:      "local-model/silero_vad.onnx",
			STTPath:      "local-model/sherpa-onnx-sense-voice-zh-en-ja-ko-yue-int8-2024-07-17",
			SpeakerPath:  "local-model/3dspeaker_speech_eres2net_base_sv_zh-cn_3dspeaker_16k.onnx",
			TTSPath:      "local-model/vits-piper-en_GB-cori-medium",
			IntentPath:   "local-model/Qwen3-0.6B-Q4_K_M.gguf",
			LlamaPath:    "llama-cli",
			Threads:      4,
			CPUOnly:      true,
		},
		HomeAssistant: HomeAssistant{
			URL:     "http://127.0.0.1:8123",
			Timeout: 10 * time.Second,
		},
		API: API{Address: "127.0.0.1:8080"},
	}, nil
}
