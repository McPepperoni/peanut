package sqlite

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"peanut/internal/models"
)

func TestModelStoreReplacesAndListsSnapshot(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "peanut.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	store := NewModelStore(db)
	profiles := []models.Profile{{ID: "intent-local", Role: models.RoleIntent, Runtime: "local", Path: "intent/local", Entry: "model.gguf", Threads: 4, Valid: true}}
	if err := store.ReplaceSnapshot(ctx, profiles); err != nil {
		t.Fatal(err)
	}

	got, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	profiles[0].Active = true
	if len(got) != 1 || got[0] != profiles[0] {
		t.Fatalf("profiles = %#v", got)
	}
}

func TestModelStoreRejectsAbsolutePaths(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "peanut.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	profile := models.Profile{ID: "intent-local", Role: models.RoleIntent, Runtime: "local", Path: filepath.Join(t.TempDir(), "intent", "local"), Entry: "model.gguf", Valid: true}
	if err := NewModelStore(db).ReplaceSnapshot(ctx, []models.Profile{profile}); err == nil {
		t.Fatal("absolute model path was stored")
	}
}

func TestModelStoreRejectsUnsafeEntries(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "peanut.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	for _, entry := range []string{filepath.Join(t.TempDir(), "model.gguf"), "../model.gguf"} {
		profile := models.Profile{ID: "intent-local", Role: models.RoleIntent, Runtime: "local", Path: "intent/local", Entry: entry, Valid: true}
		if err := NewModelStore(db).ReplaceSnapshot(ctx, []models.Profile{profile}); err == nil {
			t.Errorf("unsafe entry %q was stored", entry)
		}
	}
}

func TestFreshRegistryPreservesPersistedActiveRoleOnInvalidReload(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "peanut.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	writeManifest(t, root, `{"id":"intent-local","role":"intent","runtime":"local","entry":"model.gguf"}`)
	store := NewModelStore(db)
	if _, err := models.NewRegistry(root, store, nil).Scan(ctx); err != nil {
		t.Fatal(err)
	}
	writeManifest(t, root, `{"id":"intent-local","role":"intent","runtime":"local","entry":"missing.gguf"}`)

	registry := models.NewRegistry(root, store, nil)
	snapshot, err := registry.Scan(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if active, ok := snapshot.Active[models.RoleIntent]; !ok || active.ID != "intent-local" {
		t.Fatalf("snapshot active = %#v", snapshot.Active)
	}
	if active, ok := registry.Active(models.RoleIntent); !ok || active.ID != "intent-local" {
		t.Fatalf("registry active = %#v", active)
	}
	var activeID string
	if err := db.QueryRowContext(ctx, `SELECT id FROM models WHERE role = 'intent' AND active = 1`).Scan(&activeID); err != nil {
		t.Fatal(err)
	}
	if activeID != "intent-local" {
		t.Fatalf("stored active = %q", activeID)
	}
	stored, err := store.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 2 {
		t.Fatalf("stored profiles = %#v", stored)
	}
}

func writeManifest(t *testing.T, root, manifest string) {
	t.Helper()
	directory := filepath.Join(root, "intent", "local")
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "model.json"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "model.gguf"), []byte("model"), 0644); err != nil {
		t.Fatal(err)
	}
}
