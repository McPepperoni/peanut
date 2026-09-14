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
	"testing"

	"peanut/internal/api"
	"peanut/internal/config"
	"peanut/internal/models"
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

func TestModelsAPIReloadReachesRuntimeSwapBoundary(t *testing.T) {
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
	profileDir := filepath.Join(root, "intent", "local")
	if err := os.MkdirAll(profileDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profileDir, "model.gguf"), []byte("model"), 0644); err != nil {
		t.Fatal(err)
	}
	manifest := map[string]any{"id": "intent-live", "role": "intent", "runtime": "local", "entry": "model.gguf"}
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

	swaps := 0
	owner := newReloadableModels(func(_ context.Context, snapshot models.Snapshot) (modelSet, error) {
		swaps++
		return modelSet{parser: runtimeTestParser{language: snapshot.Active[models.RoleIntent].ID}}, nil
	})
	registry := newRuntimeModelRegistry(cfg, db, owner)
	server := api.NewServer(db, nil, registry)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/models", nil)
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusOK || swaps != 1 {
		t.Fatalf("status = %d, swaps = %d, body = %s", response.Code, swaps, response.Body.String())
	}
}
