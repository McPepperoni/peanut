package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"peanut/internal/config"
	"peanut/internal/intent"
	"peanut/internal/models"
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

	server := NewServer(db, nil, nil)
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
	server := NewServer(db, nil, nil)

	unauthorized := request(t, server.Handler(), http.MethodGet, "/api/v1/config", "", "")
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}
	authorized := request(t, server.Handler(), http.MethodGet, "/api/v1/config", "", "pairing-secret")
	if authorized.Code != http.StatusOK {
		t.Fatalf("authorized status = %d, body = %s", authorized.Code, authorized.Body.String())
	}
}

func TestServerRejectsLANAuthenticationDowngradeUntilRestart(t *testing.T) {
	db := apiDB(t)
	store := sqlite.NewConfigStore(db)
	cfg := loadConfig(t, store)
	cfg.API.Address = "0.0.0.0:8080"
	cfg.API.AllowLAN = true
	cfg.API.PairingToken = "pairing-secret"
	if err := store.Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	server := NewServer(db, nil, nil)

	for _, body := range []string{
		`{"api":{"allow_lan":false}}`,
		`{"api":{"address":"127.0.0.1:8080"}}`,
	} {
		response := request(t, server.Handler(), http.MethodPut, "/api/v1/config", body, "pairing-secret")
		if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "restart required") {
			t.Fatalf("body %s: status = %d, response = %s", body, response.Code, response.Body.String())
		}
	}
	cfg = loadConfig(t, store)
	if !cfg.API.AllowLAN || cfg.API.Address != "0.0.0.0:8080" {
		t.Fatalf("stored API config changed: %+v", cfg.API)
	}
}

func TestServerFailsClosedWhenLANPairingTokenIsEmpty(t *testing.T) {
	db := apiDB(t)
	store := sqlite.NewConfigStore(db)
	cfg := loadConfig(t, store)
	cfg.API.Address = "0.0.0.0:8080"
	cfg.API.AllowLAN = true
	cfg.API.PairingToken = ""
	if err := store.Save(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	server := NewServer(db, nil, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	req.Header.Set("Authorization", "Bearer ")
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, req)

	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "authentication unavailable") {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
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
	server := NewServer(db, nil, nil)

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
	server := NewServer(db, nil, nil)
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
	server := NewServer(db, nil, nil)
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
	server := NewServer(db, refresher, nil)

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
	server := NewServer(db, nil, nil)

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
	server := NewServer(db, refresher, nil)
	response := request(t, server.Handler(), http.MethodPost, "/api/v1/config/refresh", "", "")
	if response.Code != http.StatusNoContent || refresher.calls != 1 {
		t.Fatalf("status = %d, calls = %d, body = %s", response.Code, refresher.calls, response.Body.String())
	}
}

func TestServerReturnsJSONWhenConfigurationCannotLoad(t *testing.T) {
	db := apiDB(t)
	server := NewServer(db, nil, nil)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	response := request(t, server.Handler(), http.MethodGet, "/api/v1/config", "", "")
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type = %q", got)
	}
}

func TestModelsGETReloadsAndReturnsSnapshot(t *testing.T) {
	db := apiDB(t)
	reloader := &fakeModelReloader{snapshot: models.Snapshot{
		Profiles: []models.Profile{
			{ID: "intent-local", Role: models.RoleIntent, Valid: true},
			{ID: "invalid:stt/bad", Role: models.RoleSTT, Error: "entry missing"},
		},
		Active: map[models.Role]models.Profile{
			models.RoleIntent: {ID: "intent-local", Role: models.RoleIntent, Valid: true},
		},
	}}

	response := request(t, NewServer(db, nil, reloader).Handler(), http.MethodGet, "/api/v1/models", "", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if reloader.calls != 1 {
		t.Fatalf("reload calls = %d", reloader.calls)
	}
	var body struct {
		Profiles []models.Profile               `json:"profiles"`
		Active   map[models.Role]models.Profile `json:"active"`
		Errors   []string                       `json:"errors"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Profiles) != 2 || body.Active[models.RoleIntent].ID != "intent-local" {
		t.Fatalf("snapshot = %+v", body)
	}
	if len(body.Errors) != 1 || body.Errors[0] != "entry missing" {
		t.Fatalf("errors = %#v", body.Errors)
	}
}

func TestModelsGETKeepsPreviousSnapshotOnReloadError(t *testing.T) {
	db := apiDB(t)
	reloader := &fakeModelReloader{
		err: errors.New("bad model"),
		active: map[models.Role]models.Profile{
			models.RoleIntent: {ID: "intent-old", Role: models.RoleIntent, Valid: true},
		},
	}

	response := request(t, NewServer(db, nil, reloader).Handler(), http.MethodGet, "/api/v1/models", "", "")
	if response.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if _, ok := reloader.Active(models.RoleIntent); !ok {
		t.Fatal("active role was erased")
	}
}

func TestModelsRejectsOtherMethods(t *testing.T) {
	db := apiDB(t)
	response := request(t, NewServer(db, nil, &fakeModelReloader{}).Handler(), http.MethodPost, "/api/v1/models", "", "")
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != "GET" {
		t.Fatalf("status = %d, Allow = %q", response.Code, response.Header().Get("Allow"))
	}
}

func TestDocsEndpointsAreEmbeddedAndLocal(t *testing.T) {
	db := apiDB(t)
	handler := NewServer(db, nil, nil).Handler()

	openAPI := request(t, handler, http.MethodGet, "/api/v1/openapi.json", "", "")
	if openAPI.Code != http.StatusOK || openAPI.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("openapi status = %d, content type = %q", openAPI.Code, openAPI.Header().Get("Content-Type"))
	}
	var document struct {
		OpenAPI string         `json:"openapi"`
		Paths   map[string]any `json:"paths"`
	}
	if err := json.NewDecoder(openAPI.Body).Decode(&document); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/api/v1/config", "/api/v1/config/pairing-token", "/api/v1/config/refresh", "/api/v1/models", "/api/v1/openapi.json", "/docs"} {
		if _, ok := document.Paths[path]; !ok {
			t.Errorf("OpenAPI missing path %q", path)
		}
	}

	docs := request(t, handler, http.MethodGet, "/docs", "", "")
	if docs.Code != http.StatusOK || docs.Header().Get("Content-Type") != "text/html; charset=utf-8" {
		t.Fatalf("docs status = %d, content type = %q", docs.Code, docs.Header().Get("Content-Type"))
	}
	html := docs.Body.String()
	if !strings.Contains(html, "/api/v1/openapi.json") || !strings.Contains(html, `src="/scalar.js"`) {
		t.Fatalf("docs HTML = %s", html)
	}
	if strings.Contains(html, `src="http://`) || strings.Contains(html, `src="https://`) {
		t.Fatalf("docs use remote script source: %s", html)
	}

	script := request(t, handler, http.MethodGet, "/scalar.js", "", "")
	if script.Code != http.StatusOK || script.Body.Len() == 0 {
		t.Fatalf("scalar script status = %d, size = %d", script.Code, script.Body.Len())
	}
}

type fakeRefresher struct{ calls int }

func (f *fakeRefresher) Discover(context.Context) ([]intent.Capability, error) {
	f.calls++
	return nil, nil
}

type fakeModelReloader struct {
	snapshot models.Snapshot
	err      error
	calls    int
	active   map[models.Role]models.Profile
}

func (f *fakeModelReloader) Reload(context.Context) (models.Snapshot, error) {
	f.calls++
	return f.snapshot, f.err
}

func (f *fakeModelReloader) Active(role models.Role) (models.Profile, bool) {
	profile, ok := f.active[role]
	return profile, ok
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
