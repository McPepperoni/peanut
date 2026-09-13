package sqlite

import (
	"context"
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
