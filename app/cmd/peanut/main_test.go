package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"peanut/internal/api"
	"peanut/internal/config"
	"peanut/internal/storage/sqlite"
)

func TestRunMainInitializesDatabaseAndDispatchesArguments(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "peanut.db")
	ran := false
	err := runMain(context.Background(), []string{"peanut", "run"}, databasePath, commandDependencies{
		Run: func(context.Context) error {
			ran = true
			return nil
		},
	}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !ran {
		t.Fatal("run command was not dispatched")
	}
	if _, err := config.Load(databasePath); err != nil {
		t.Fatalf("load initialized database: %v", err)
	}
}

func TestRunMainWithoutArgumentsReturnsUsage(t *testing.T) {
	err := runMain(context.Background(), nil, filepath.Join(t.TempDir(), "peanut.db"), commandDependencies{}, io.Discard)
	if err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("error = %v, want usage", err)
	}
}

func TestModelsAPIUsesDiscoveryWithoutRuntimeConstruction(t *testing.T) {
	ctx := context.Background()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "peanut.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := config.PersistDefaults(ctx, db); err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	profileDir := filepath.Join(root, "stt", "local")
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "model.gguf"), []byte("model"), 0644); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{"id": "stt-live", "role": "stt", "runtime": "local", "entry": "model.gguf"}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "model.json"), manifestBytes, 0644); err != nil {
		t.Fatal(err)
	}

	var cfg config.Config
	store := sqlite.NewConfigStore(db)
	if err := store.Load(ctx, &cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Models.Root = root
	if err := store.Save(ctx, cfg); err != nil {
		t.Fatal(err)
	}

	registry := newAPIModelRegistry(cfg, db)
	server := api.NewServer(db, nil, registry)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/models", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	stored, err := sqlite.NewModelStore(db).List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 1 || stored[0].ID != "stt-live" {
		t.Fatalf("stored models = %#v", stored)
	}
}

func TestTerminationSignalsIncludeInterruptAndSIGTERM(t *testing.T) {
	signals := terminationSignals()
	for _, want := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		found := false
		for _, signal := range signals {
			found = found || signal == want
		}
		if !found {
			t.Errorf("termination signals %v do not include %v", signals, want)
		}
	}
}
