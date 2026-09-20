package config

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"peanut/internal/storage/sqlite"
)

func TestLoadDefaultsAreValid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "peanut.db")
	db := initializeDatabase(t, path)
	if err := PersistDefaults(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabasePath == "" {
		t.Fatal("database path is empty")
	}
	if cfg.Audio.SampleRate != 16000 || cfg.Audio.Channels != 1 || cfg.Audio.FrameSamples != 320 {
		t.Fatalf("unexpected normalized audio config: %+v", cfg.Audio)
	}
	if cfg.Audio.PreRollMilliseconds != 300 {
		t.Fatalf("pre-roll = %d ms, want 300 ms", cfg.Audio.PreRollMilliseconds)
	}
	if cfg.HomeAssistant.URL != "http://127.0.0.1:8123" {
		t.Fatalf("Home Assistant URL = %q", cfg.HomeAssistant.URL)
	}
	if cfg.API.Address != "127.0.0.1:8080" {
		t.Fatalf("API address = %q", cfg.API.Address)
	}
	if cfg.Models.Root != "models" || cfg.Models.IntentModel != "functiongemma.gguf" || cfg.Models.Threads != 4 || !cfg.Models.CPUOnly {
		t.Fatalf("model config = %+v", cfg.Models)
	}
}

func TestPersistDefaultsDoesNotReplaceStoredConfig(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "peanut.db")
	db := initializeDatabase(t, path)
	if err := PersistDefaults(ctx, db); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.HomeAssistant.URL = "http://homeassistant.local:8123"
	stored, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE settings SET value = ? WHERE key = 'runtime_config'`, stored); err != nil {
		t.Fatal(err)
	}
	if err := PersistDefaults(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	cfg, err = Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HomeAssistant.URL != "http://homeassistant.local:8123" {
		t.Fatalf("Home Assistant URL after reopen = %q", cfg.HomeAssistant.URL)
	}
}

func TestPersistDefaultsMigratesLegacyModelPaths(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "peanut.db")
	db := initializeDatabase(t, path)
	legacy := struct {
		Audio  Audio
		Models struct {
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
		HomeAssistant HomeAssistant
		API           API
	}{
		Audio:         defaultConfig().Audio,
		HomeAssistant: defaultConfig().HomeAssistant,
		API:           defaultConfig().API,
	}
	legacy.Audio.InputDevice = "legacy-mic"
	legacy.HomeAssistant.Token = "ha-secret"
	legacy.API.PairingToken = "pairing-secret"
	legacy.Models.WakeWordPath = "local-model/kws"
	legacy.Models.VADPath = "local-model/vad.onnx"
	legacy.Models.STTPath = "local-model/stt"
	legacy.Models.SpeakerPath = "local-model/speaker.onnx"
	legacy.Models.TTSPath = "local-model/tts"
	legacy.Models.IntentPath = "local-model/intent.gguf"
	legacy.Models.LlamaPath = "llama-cli"
	legacy.Models.Threads = 2
	legacy.Models.CPUOnly = true
	stored, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ('runtime_config', ?)`, stored); err != nil {
		t.Fatal(err)
	}
	if err := PersistDefaults(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Models.Root != "models" || cfg.Models.Threads != 2 || !cfg.Models.CPUOnly {
		t.Fatalf("model config = %+v", cfg.Models)
	}
	if cfg.Audio.InputDevice != "legacy-mic" || cfg.HomeAssistant.Token != "ha-secret" || cfg.API.PairingToken != "pairing-secret" {
		t.Fatalf("unrelated settings changed: %+v", cfg)
	}
}

func TestPersistDefaultsMigratesMissingIntentModel(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "peanut.db")
	db := initializeDatabase(t, path)
	cfg := defaultConfig()
	cfg.Models.Root = "custom-models"
	cfg.Models.IntentModel = ""
	cfg.HomeAssistant.Token = "ha-secret"
	cfg.API.PairingToken = "pairing-secret"
	stored, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ('runtime_config', ?)`, stored); err != nil {
		t.Fatal(err)
	}
	if err := PersistDefaults(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Models.IntentModel != "functiongemma.gguf" {
		t.Fatalf("intent model = %q", got.Models.IntentModel)
	}
	if got.Models.Root != "custom-models" || got.HomeAssistant.Token != "ha-secret" || got.API.PairingToken != "pairing-secret" {
		t.Fatalf("unrelated settings changed: %+v", got)
	}
}

func TestLoadRejectsInvalidStoredConfig(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "peanut.db")
	db := initializeDatabase(t, path)
	if err := PersistDefaults(ctx, db); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Audio.AcknowledgementTail = -1
	stored, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `UPDATE settings SET value = ? WHERE key = 'runtime_config'`, stored); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("Load accepted invalid stored config")
	}
}

func TestLoadRejectsEmptyDatabasePath(t *testing.T) {
	if _, err := Load(""); err == nil {
		t.Fatal("Load accepted an empty database path")
	}
}

func initializeDatabase(t *testing.T, path string) *sqlite.DB {
	t.Helper()
	db, err := sqlite.Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(context.Background()); err != nil {
		db.Close()
		t.Fatal(err)
	}
	return db
}
