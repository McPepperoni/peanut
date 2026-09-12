package config

import "testing"

func TestLoadDefaultsAreValid(t *testing.T) {
	cfg, err := Load(t.TempDir() + "/peanut.db")
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

func TestLoadRejectsEmptyDatabasePath(t *testing.T) {
	if _, err := Load(""); err == nil {
		t.Fatal("Load accepted an empty database path")
	}
}
