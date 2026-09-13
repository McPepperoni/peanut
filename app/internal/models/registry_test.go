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

type fakeModelStore struct{ profiles []Profile }

func (s *fakeModelStore) ReplaceSnapshot(_ context.Context, profiles []Profile) error {
	s.profiles = append([]Profile(nil), profiles...)
	return nil
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
		valid[profile.ID] = profile.Valid
	}
	if !valid["good"] || valid["bad-checksum"] || valid["unknown-field"] {
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
	registry := NewRegistry(root, &fakeModelStore{}, nil)
	if _, err := registry.Scan(context.Background()); err != nil {
		t.Fatal(err)
	}
	registry.swap = func(context.Context, Snapshot) error { return errors.New("load failed") }

	if _, err := registry.Scan(context.Background()); err == nil {
		t.Fatal("want swap error")
	}
	if _, ok := registry.Active(RoleIntent); !ok {
		t.Fatal("prior role was discarded")
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
