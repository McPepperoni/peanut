package models

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

type fakeModelStore struct {
	profiles []Profile
	err      error
}

func (s *fakeModelStore) ReplaceSnapshot(_ context.Context, profiles []Profile) error {
	if s.err != nil {
		return s.err
	}
	seen := make(map[string]bool)
	for _, profile := range profiles {
		if seen[profile.ID] {
			return fmt.Errorf("duplicate id %q", profile.ID)
		}
		seen[profile.ID] = true
	}
	s.profiles = append([]Profile(nil), profiles...)
	return nil
}

func (s *fakeModelStore) List(context.Context) ([]Profile, error) {
	return append([]Profile(nil), s.profiles...), nil
}

func TestRegistryScansManifestProfiles(t *testing.T) {
	root := t.TempDir()
	writeModelManifest(t, root, "intent/local", `{"id":"intent-local","role":"intent","runtime":"local","entry":"model.gguf","sha256":""}`)
	registry := NewRegistry(root, &fakeModelStore{}, nil)

	snapshot, err := registry.Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := snapshot.Profiles[0].ID; got != "intent-local" {
		t.Fatalf("id = %q", got)
	}
	if !snapshot.Profiles[0].Valid {
		t.Fatalf("profile invalid: %s", snapshot.Profiles[0].Error)
	}
}

func TestRegistryRejectsPathOutsideRoot(t *testing.T) {
	root := t.TempDir()
	writeModelManifest(t, root, "stt/bad", `{"id":"bad","role":"stt","runtime":"local","entry":"../../secret.onnx"}`)

	snapshot, err := NewRegistry(root, &fakeModelStore{}, nil).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Profiles[0].Valid {
		t.Fatal("path traversal marked valid")
	}
}

func TestRegistryValidatesStrictManifestAndChecksum(t *testing.T) {
	root := t.TempDir()
	sum := sha256.Sum256([]byte("model"))
	writeModelManifest(t, root, "intent/good", fmt.Sprintf(`{"id":"good","role":"intent","runtime":"local","entry":"model.gguf","sha256":"%X"}`, sum))
	writeModelManifest(t, root, "intent/bad-checksum", `{"id":"bad-checksum","role":"intent","runtime":"local","entry":"model.gguf","sha256":"00"}`)
	writeModelManifest(t, root, "intent/unknown-field", `{"id":"unknown-field","role":"intent","runtime":"local","entry":"model.gguf","extra":true}`)

	snapshot, err := NewRegistry(root, &fakeModelStore{}, nil).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	valid := map[string]bool{}
	for _, profile := range snapshot.Profiles {
		valid[profile.Path] = profile.Valid
	}
	if !valid["intent/good"] || valid["intent/bad-checksum"] || valid["intent/unknown-field"] {
		t.Fatalf("validity = %#v", valid)
	}
}

func TestRegistryScansOnlyDirectProfilesAndSortsByID(t *testing.T) {
	root := t.TempDir()
	writeModelManifest(t, root, "intent/z", `{"id":"z","role":"intent","runtime":"local","entry":"model.gguf"}`)
	writeModelManifest(t, root, "intent/a", `{"id":"a","role":"intent","runtime":"local","entry":"model.gguf"}`)
	writeModelManifest(t, root, "intent/a/nested", `{"id":"nested","role":"intent","runtime":"local","entry":"model.gguf"}`)

	snapshot, err := NewRegistry(root, &fakeModelStore{}, nil).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Profiles) != 2 || snapshot.Profiles[0].ID != "a" || snapshot.Profiles[1].ID != "z" {
		t.Fatalf("profiles = %#v", snapshot.Profiles)
	}
}

func TestRegistryKeepsPriorRoleWhenReloadFails(t *testing.T) {
	root := t.TempDir()
	writeModelManifest(t, root, "intent/local", `{"id":"intent-local","role":"intent","runtime":"local","entry":"model.gguf"}`)
	store := &fakeModelStore{}
	registry := NewRegistry(root, store, nil)
	if _, err := registry.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	writeModelManifest(t, root, "intent/local", `{"id":"intent-new","role":"intent","runtime":"local","entry":"model.gguf"}`)
	registry.swap = func(context.Context, Snapshot) error { return errors.New("load failed") }

	if _, err := registry.Scan(context.Background()); err == nil {
		t.Fatal("want swap error")
	}
	if active, ok := registry.Active(RoleIntent); !ok || active.ID != "intent-local" {
		t.Fatalf("active intent = %#v", active)
	}
	if len(store.profiles) != 1 || store.profiles[0].ID != "intent-local" {
		t.Fatalf("stored profiles = %#v", store.profiles)
	}
}

func TestFreshRegistryRejectsPersistedActiveProfileWithInvalidChecksum(t *testing.T) {
	root := t.TempDir()
	sum := sha256.Sum256([]byte("model"))
	writeModelManifest(t, root, "intent/local", fmt.Sprintf(`{"id":"intent-local","role":"intent","runtime":"local","entry":"model.gguf","sha256":"%x"}`, sum))
	store := &fakeModelStore{}
	if _, err := NewRegistry(root, store, nil).Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "intent", "local", "model.gguf"), []byte("corrupt"), 0644); err != nil {
		t.Fatal(err)
	}

	snapshot, err := NewRegistry(root, store, nil).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := snapshot.Active[RoleIntent]; ok {
		t.Fatalf("stale active profile = %#v", snapshot.Active)
	}
}

func TestRegistryDoesNotSwapWhenPersistenceFails(t *testing.T) {
	root := t.TempDir()
	writeModelManifest(t, root, "intent/local", `{"id":"intent-local","role":"intent","runtime":"local","entry":"model.gguf"}`)
	store := &fakeModelStore{}
	registry := NewRegistry(root, store, nil)
	if _, err := registry.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	writeModelManifest(t, root, "intent/local", `{"id":"intent-new","role":"intent","runtime":"local","entry":"model.gguf"}`)
	store.err = errors.New("write failed")
	swapCalls := 0
	registry.swap = func(context.Context, Snapshot) error {
		swapCalls++
		return nil
	}

	if _, err := registry.Scan(context.Background()); err == nil {
		t.Fatal("want persistence error")
	}
	if swapCalls != 0 {
		t.Fatalf("swap calls = %d", swapCalls)
	}
	if active, ok := registry.Active(RoleIntent); !ok || active.ID != "intent-local" {
		t.Fatalf("active intent = %#v", active)
	}
	if len(store.profiles) != 1 || store.profiles[0].ID != "intent-local" {
		t.Fatalf("stored profiles = %#v", store.profiles)
	}
}

func TestRegistryAssignsUniqueIDsToInvalidProfiles(t *testing.T) {
	root := t.TempDir()
	writeModelManifest(t, root, "intent/first", `{"role":"intent","runtime":"local","entry":"model.gguf"}`)
	writeModelManifest(t, root, "intent/second", `{"role":"intent","runtime":"local","entry":"model.gguf"}`)
	store := &fakeModelStore{}

	snapshot, err := NewRegistry(root, store, nil).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Profiles) != 2 || snapshot.Profiles[0].ID == "" || snapshot.Profiles[0].ID == snapshot.Profiles[1].ID {
		t.Fatalf("profiles = %#v", snapshot.Profiles)
	}
	for _, profile := range snapshot.Profiles {
		if profile.Valid {
			t.Fatalf("missing-id profile marked valid: %#v", profile)
		}
	}
}

func TestRegistryResolvesManifestAndSyntheticIDCollisions(t *testing.T) {
	root := t.TempDir()
	writeModelManifest(t, root, "intent/missing", `{"role":"intent","runtime":"local","entry":"model.gguf"}`)
	writeModelManifest(t, root, "tts/claimed", `{"id":"invalid:intent/missing","role":"tts","runtime":"local","entry":"model.gguf"}`)
	writeModelManifest(t, root, "stt/first", `{"id":"duplicate","role":"stt","runtime":"local","entry":"model.gguf"}`)
	writeModelManifest(t, root, "vad/second", `{"id":"duplicate","role":"vad","runtime":"local","entry":"model.gguf"}`)
	store := &fakeModelStore{}

	snapshot, err := NewRegistry(root, store, nil).Scan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]bool)
	for _, profile := range snapshot.Profiles {
		if profile.ID == "" || seen[profile.ID] {
			t.Fatalf("colliding profile identity: %#v", snapshot.Profiles)
		}
		seen[profile.ID] = true
		if profile.Path == "stt/first" || profile.Path == "vad/second" {
			if profile.Valid {
				t.Fatalf("duplicate supplied ID remained valid: %#v", profile)
			}
		}
	}
	if len(store.profiles) != 4 {
		t.Fatalf("stored profiles = %#v", store.profiles)
	}
}

func writeModelManifest(t *testing.T, root, relative, manifest string) {
	t.Helper()
	directory := filepath.Join(root, filepath.FromSlash(relative))
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
