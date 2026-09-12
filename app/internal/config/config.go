package config

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"time"

	"peanut/internal/storage/sqlite"
)

const DefaultDatabasePath = "data/peanut.db"

type Config struct {
	DatabasePath  string `json:"-"`
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
	db, err := sqlite.Open(context.Background(), path)
	if err != nil {
		return Config{}, err
	}
	defer db.Close()
	var stored []byte
	if err := db.QueryRow(`SELECT value FROM settings WHERE key = 'runtime_config'`).Scan(&stored); err != nil {
		return Config{}, fmt.Errorf("load runtime config: %w", err)
	}
	var cfg Config
	if err := json.Unmarshal(stored, &cfg); err != nil {
		return Config{}, fmt.Errorf("decode runtime config: %w", err)
	}
	cfg.DatabasePath = path
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func PersistDefaults(ctx context.Context, db *sqlite.DB) error {
	stored, err := json.Marshal(defaultConfig())
	if err != nil {
		return fmt.Errorf("encode default config: %w", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT OR IGNORE INTO settings (key, value) VALUES ('runtime_config', ?)`, stored); err != nil {
		return fmt.Errorf("persist default config: %w", err)
	}
	return nil
}

func defaultConfig() Config {
	return Config{
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
	}
}

func (cfg Config) validate() error {
	if cfg.DatabasePath == "" {
		return errors.New("database path is required")
	}
	if cfg.Audio.SampleRate <= 0 || cfg.Audio.Channels <= 0 || cfg.Audio.FrameSamples <= 0 || cfg.Audio.PreRollMilliseconds < 0 {
		return errors.New("invalid audio format")
	}
	if cfg.Audio.AcknowledgementTail < 0 || cfg.Audio.PlaybackTail < 0 || cfg.Audio.VADThreshold <= 0 || cfg.Audio.VADThreshold > 1 || cfg.Audio.NoSpeechTimeout <= 0 || cfg.Audio.MaximumCommandDuration <= 0 {
		return errors.New("invalid audio detection config")
	}
	if cfg.Models.WakeWordPath == "" || cfg.Models.VADPath == "" || cfg.Models.STTPath == "" || cfg.Models.SpeakerPath == "" || cfg.Models.TTSPath == "" || cfg.Models.IntentPath == "" || cfg.Models.LlamaPath == "" || cfg.Models.Threads <= 0 || !cfg.Models.CPUOnly {
		return errors.New("invalid model config")
	}
	haURL, err := url.ParseRequestURI(cfg.HomeAssistant.URL)
	if err != nil || haURL.Host == "" || (haURL.Scheme != "http" && haURL.Scheme != "https") || cfg.HomeAssistant.Timeout <= 0 {
		return errors.New("invalid Home Assistant config")
	}
	if _, _, err := net.SplitHostPort(cfg.API.Address); err != nil {
		return errors.New("invalid API address")
	}
	if cfg.API.AllowLAN && cfg.API.PairingToken == "" {
		return errors.New("LAN API requires pairing token")
	}
	if cfg.Audio.DebugAudio && cfg.Audio.DebugAudioPath == "" {
		return errors.New("debug audio path is required")
	}
	return nil
}
