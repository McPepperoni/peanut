package providers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"peanut/internal/config"
	"peanut/internal/intent"
	"peanut/internal/storage/sqlite"
)

func TestHomeAssistantDiscoverAuthenticatesAndCachesMediaPlayers(t *testing.T) {
	const token = "test-token"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/states" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[
			{"entity_id":"media_player.living_room","state":"idle","attributes":{"friendly_name":"Living room speaker"}},
			{"entity_id":"sensor.temperature","state":"21","attributes":{"friendly_name":"Temperature"}}
		]`))
	}))

	db := homeAssistantDB(t, server.URL, token)
	provider := NewHomeAssistantProvider(db, server.Client())
	capabilities, err := provider.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(capabilities) != 1 {
		t.Fatalf("capabilities = %#v", capabilities)
	}
	capability := capabilities[0]
	if capability.ProviderID != HomeAssistantProviderID || capability.DeviceID != "media_player.living_room" || capability.Name != "Living room speaker" {
		t.Fatalf("capability = %#v", capability)
	}
	if len(capability.Actions) != 1 || capability.Actions[0].ID != "music.play" {
		t.Fatalf("actions = %#v", capability.Actions)
	}

	server.Close()
	cached, err := provider.Discover(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(cached) != 1 || cached[0].DeviceID != capability.DeviceID {
		t.Fatalf("cached capabilities = %#v", cached)
	}
}

func TestHomeAssistantExecuteMapsValidatedMusicPlay(t *testing.T) {
	const token = "test-token"
	requests := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/services/media_player/play_media" || r.Method != http.MethodPost {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+token {
			t.Fatalf("Authorization = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		requests <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	db := homeAssistantDB(t, server.URL, token)
	provider := NewHomeAssistantProvider(db, server.Client())
	_, err := provider.Execute(context.Background(), intent.ActionRequest{
		DeviceID:  "media_player.kitchen",
		ActionID:  "music.play",
		Arguments: map[string]any{"query": "Blue in Green", "source": "music"},
	})
	if err != nil {
		t.Fatal(err)
	}
	body := <-requests
	if body["entity_id"] != "media_player.kitchen" || body["media_content_id"] != "Blue in Green" || body["media_content_type"] != "music" {
		t.Fatalf("service body = %#v", body)
	}
}

func TestHomeAssistantRejectsUnvalidatedAction(t *testing.T) {
	db := homeAssistantDB(t, "http://127.0.0.1:1", "test-token")
	provider := NewHomeAssistantProvider(db, nil)
	_, err := provider.Execute(context.Background(), intent.ActionRequest{
		DeviceID: "light.kitchen", ActionID: "music.play", Arguments: map[string]any{"query": "x"},
	})
	if !errors.Is(err, ErrInvalidAction) {
		t.Fatalf("error = %v, want invalid action", err)
	}
}

func TestHomeAssistantClassifiesUnavailableAndUnauthorized(t *testing.T) {
	t.Run("unavailable", func(t *testing.T) {
		db := homeAssistantDB(t, "http://127.0.0.1:1", "test-token")
		provider := NewHomeAssistantProvider(db, nil)
		if _, err := provider.Discover(context.Background()); !errors.Is(err, ErrProviderUnavailable) {
			t.Fatalf("error = %v, want provider unavailable", err)
		}
	})

	t.Run("unauthorized", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		}))
		defer server.Close()
		db := homeAssistantDB(t, server.URL, "bad-token")
		provider := NewHomeAssistantProvider(db, server.Client())
		if _, err := provider.Discover(context.Background()); !errors.Is(err, ErrProviderUnauthorized) {
			t.Fatalf("error = %v, want provider unauthorized", err)
		}
	})
}

func homeAssistantDB(t *testing.T, url, token string) *sqlite.DB {
	t.Helper()
	ctx := context.Background()
	db, err := sqlite.Open(ctx, filepath.Join(t.TempDir(), "peanut.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := config.PersistDefaults(ctx, db); err != nil {
		t.Fatal(err)
	}
	store := sqlite.NewConfigStore(db)
	var cfg config.Config
	if err := store.Load(ctx, &cfg); err != nil {
		t.Fatal(err)
	}
	cfg.HomeAssistant.URL = url
	cfg.HomeAssistant.Token = token
	cfg.HomeAssistant.Timeout = 250 * time.Millisecond
	if err := store.Save(ctx, cfg); err != nil {
		t.Fatal(err)
	}
	return db
}
