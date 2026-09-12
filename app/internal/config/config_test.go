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
