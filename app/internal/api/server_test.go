package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"peanut/internal/config"
	"peanut/internal/intent"
	"peanut/internal/storage/sqlite"
)

func TestServerDefaultsToLocalhostAndRedactsSecrets(t *testing.T) {
	db := apiDB(t)
	store := sqlite.NewConfigStore(db)
	cfg := loadConfig(t, store)
	cfg.HomeAssistant.Token = "ha-secret"
	cfg.API.PairingToken = "pairing-secret"
	if err := store.Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}

	server := NewServer(db, nil)
	address, err := server.Address(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if address != "127.0.0.1:8080" {
		t.Fatalf("address = %q", address)
	}

	response := request(t, server.Handler(), http.MethodGet, "/api/v1/config", "", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	if strings.Contains(body, "ha-secret") || strings.Contains(body, "pairing-secret") {
		t.Fatalf("response leaked secret: %s", body)
	}
	if strings.Count(body, Redacted) != 2 {
		t.Fatalf("response = %s", body)
	}
}

func TestServerRequiresBearerAuthWhenLANEnabled(t *testing.T) {
	db := apiDB(t)
	store := sqlite.NewConfigStore(db)
	cfg := loadConfig(t, store)
	cfg.API.Address = "0.0.0.0:8080"
	cfg.API.AllowLAN = true
	cfg.API.PairingToken = "pairing-secret"
	if err := store.Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	server := NewServer(db, nil)

	unauthorized := request(t, server.Handler(), http.MethodGet, "/api/v1/config", "", "")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}
	authorized := request(t, server.Handler(), http.MethodGet, "/api/v1/config", "", "pairing-secret")
	if authorized.Code != http.StatusOK {
		t.Fatalf("authorized status = %d, body = %s", authorized.Code, authorized.Body.String())
	}
}

func TestServerKeepsStartupAuthenticationAfterAllowLANUpdate(t *testing.T) {
	db := apiDB(t)
	store := sqlite.NewConfigStore(db)
	cfg := loadConfig(t, store)
	cfg.API.Address = "0.0.0.0:8080"
	cfg.API.AllowLAN = true
	cfg.API.PairingToken = "pairing-secret"
	if err := store.Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	server := NewServer(db, nil)

	updated := request(t, server.Handler(), http.MethodPut, "/api/v1/config", `{"api":{"address":"127.0.0.1:8080","allow_lan":false}}`, "pairing-secret")
	if updated.Code != http.StatusOK {
		t.Fatalf("update status = %d, body = %s", updated.Code, updated.Body.String())
	}
	unauthorized := request(t, server.Handler(), http.MethodGet, "/api/v1/config", "", "")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}
	authorized := request(t, server.Handler(), http.MethodGet, "/api/v1/config", "", "pairing-secret")
	if authorized.Code != http.StatusOK {
		t.Fatalf("authorized status = %d, body = %s", authorized.Code, authorized.Body.String())
	}
}

func TestServerConfigPutCannotOverwritePairingToken(t *testing.T) {
	db := apiDB(t)
	store := sqlite.NewConfigStore(db)
	cfg := loadConfig(t, store)
	cfg.HomeAssistant.Token = "ha-secret"
	cfg.API.PairingToken = "pairing-secret"
	if err := store.Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	server := NewServer(db, nil)

	get := request(t, server.Handler(), http.MethodGet, "/api/v1/config", "", "")
	put := request(t, server.Handler(), http.MethodPut, "/api/v1/config", get.Body.String(), "")
	if put.Code != http.StatusOK {
		t.Fatalf("GET->PUT status = %d, body = %s", put.Code, put.Body.String())
	}
	if got := loadConfig(t, store).API.PairingToken; got != "pairing-secret" {
		t.Fatalf("pairing token = %q", got)
	}
	if got := loadConfig(t, store).HomeAssistant.Token; got != "ha-secret" {
		t.Fatalf("Home Assistant token = %q", got)
	}

	put = request(t, server.Handler(), http.MethodPut, "/api/v1/config", `{"api":{"pairing_token":"attacker-controlled"}}`, "")
	if put.Code != http.StatusBadRequest {
		t.Fatalf("pairing_token status = %d, body = %s", put.Code, put.Body.String())
	}
	if got := loadConfig(t, store).API.PairingToken; got != "pairing-secret" {
		t.Fatalf("pairing token = %q", got)
	}
}

func TestServerRejectsLANAddressWithoutLANOptIn(t *testing.T) {
	db := apiDB(t)
	server := NewServer(db, nil)
	response := request(t, server.Handler(), http.MethodPut, "/api/v1/config", `{"api":{"address":"0.0.0.0:8080"}}`, "")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	cfg := loadConfig(t, sqlite.NewConfigStore(db))
	if cfg.API.Address != "127.0.0.1:8080" {
		t.Fatalf("invalid LAN address was stored: %q", cfg.API.Address)
	}
}

func TestServerGeneratesAndStoresPairingToken(t *testing.T) {
	db := apiDB(t)
	server := NewServer(db, nil)
	response := request(t, server.Handler(), http.MethodPost, "/api/v1/config/pairing-token", "", "")
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var result struct {
		PairingToken string `json:"pairing_token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if len(result.PairingToken) < 32 {
		t.Fatalf("pairing token = %q", result.PairingToken)
	}
	cfg := loadConfig(t, sqlite.NewConfigStore(db))
	if cfg.API.PairingToken != result.PairingToken {
		t.Fatal("generated pairing token was not stored")
	}
}

func TestServerConfigWriteIsValidatedAtomicAndRediscovered(t *testing.T) {
	db := apiDB(t)
	refresher := &fakeRefresher{}
	server := NewServer(db, refresher)

	invalid := request(t, server.Handler(), http.MethodPut, "/api/v1/config", `{"home_assistant":{"url":"not-a-url","token":"new-secret"}}`, "")
	if invalid.Code != http.StatusBadRequest {
		t.Fatalf("invalid status = %d, body = %s", invalid.Code, invalid.Body.String())
	}
	cfg := loadConfig(t, sqlite.NewConfigStore(db))
	if cfg.HomeAssistant.URL != "http://127.0.0.1:8123" || cfg.HomeAssistant.Token != "" {
		t.Fatalf("invalid write changed config: %+v", cfg.HomeAssistant)
	}
	if refresher.calls != 0 {
		t.Fatalf("refresh calls after invalid write = %d", refresher.calls)
	}

	valid := request(t, server.Handler(), http.MethodPut, "/api/v1/config", `{"home_assistant":{"url":"http://ha.example:8123","token":"new-secret"}}`, "")
	if valid.Code != http.StatusOK {
		t.Fatalf("valid status = %d, body = %s", valid.Code, valid.Body.String())
	}
	cfg = loadConfig(t, sqlite.NewConfigStore(db))
	if cfg.HomeAssistant.URL != "http://ha.example:8123" || cfg.HomeAssistant.Token != "new-secret" {
		t.Fatalf("stored config = %+v", cfg.HomeAssistant)
	}
	if refresher.calls != 1 {
		t.Fatalf("refresh calls = %d", refresher.calls)
	}
	if strings.Contains(valid.Body.String(), "new-secret") {
		t.Fatalf("response leaked secret: %s", valid.Body.String())
	}
}

func TestServerConfigPutPreservesRedactedHomeAssistantToken(t *testing.T) {
	db := apiDB(t)
	store := sqlite.NewConfigStore(db)
	cfg := loadConfig(t, store)
	cfg.HomeAssistant.Token = "old-secret"
	if err := store.Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	server := NewServer(db, nil)

	response := request(t, server.Handler(), http.MethodPut, "/api/v1/config", `{"home_assistant":{"token":"[REDACTED]"}}`, "")
	if response.Code != http.StatusOK {
		t.Fatalf("redacted update status = %d, body = %s", response.Code, response.Body.String())
	}
	if got := loadConfig(t, store).HomeAssistant.Token; got != "old-secret" {
		t.Fatalf("token after redacted update = %q", got)
	}

	response = request(t, server.Handler(), http.MethodPut, "/api/v1/config", `{"home_assistant":{"token":"new-secret"}}`, "")
	if response.Code != http.StatusOK {
		t.Fatalf("replacement update status = %d, body = %s", response.Code, response.Body.String())
	}
	if got := loadConfig(t, store).HomeAssistant.Token; got != "new-secret" {
		t.Fatalf("token after replacement update = %q", got)
	}
}

func TestServerRefreshEndpointRediscoversCapabilities(t *testing.T) {
	db := apiDB(t)
	refresher := &fakeRefresher{}
	server := NewServer(db, refresher)
	response := request(t, server.Handler(), http.MethodPost, "/api/v1/config/refresh", "", "")
	if response.Code != http.StatusNoContent || refresher.calls != 1 {
		t.Fatalf("status = %d, calls = %d, body = %s", response.Code, refresher.calls, response.Body.String())
	}
}

type fakeRefresher struct{ calls int }

func (f *fakeRefresher) Discover(context.Context) ([]intent.Capability, error) {
	f.calls++
	return nil, nil
}

func apiDB(t *testing.T) *sqlite.DB {
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
	return db
}

func loadConfig(t *testing.T, store *sqlite.ConfigStore) config.Config {
	t.Helper()
	var cfg config.Config
	if err := store.Load(context.Background(), &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func request(t *testing.T, handler http.Handler, method, path, body, token string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, req)
	return response
}
