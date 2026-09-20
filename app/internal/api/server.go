package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"peanut/internal/config"
	"peanut/internal/intent"
	"peanut/internal/logging"
	"peanut/internal/models"
	"peanut/internal/storage/sqlite"
)

const Redacted = "[REDACTED]"

var errConfigUnavailable = errors.New("configuration unavailable")

//go:embed openapi.json
var openAPIDocument []byte

//go:embed docs.html
var docsPage []byte

//go:embed scalar.js
var scalarScript []byte

type Refresher interface {
	Discover(context.Context) ([]intent.Capability, error)
}

type ModelReloader interface {
	Reload(context.Context) (models.Snapshot, error)
}

type Server struct {
	config        *sqlite.ConfigStore
	refresher     Refresher
	modelReloader ModelReloader
	logger        *slog.Logger
	handler       http.Handler
	boundLAN      bool
	configMu      sync.Mutex
}

func NewServer(db *sqlite.DB, refresher Refresher, modelReloader ModelReloader) *Server {
	return NewServerWithLogger(db, refresher, modelReloader, nil)
}

func NewServerWithLogger(db *sqlite.DB, refresher Refresher, modelReloader ModelReloader, logger *slog.Logger) *Server {
	server := &Server{
		config:        sqlite.NewConfigStore(db),
		refresher:     refresher,
		modelReloader: modelReloader,
		logger:        logging.Normalize(logger),
	}
	if cfg, err := server.load(context.Background()); err == nil {
		server.boundLAN = cfg.API.AllowLAN || !isLoopbackAddress(cfg.API.Address)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/config", server.handleConfig)
	mux.HandleFunc("/api/v1/config/pairing-token", server.handlePairingToken)
	mux.HandleFunc("/api/v1/config/refresh", server.handleRefresh)
	mux.HandleFunc("/api/v1/models", server.handleModels)
	mux.HandleFunc("/api/v1/openapi.json", serveEmbedded("application/json", openAPIDocument))
	mux.HandleFunc("/docs", serveEmbedded("text/html; charset=utf-8", docsPage))
	mux.HandleFunc("/scalar.js", serveEmbedded("text/javascript; charset=utf-8", scalarScript))
	server.handler = server.requestLogger(server.authenticate(mux))
	return server
}

func (s *Server) requestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		response := &loggingResponseWriter{ResponseWriter: w}
		next.ServeHTTP(response, r)

		status := response.status
		if status == 0 {
			status = http.StatusOK
		}
		route := r.Pattern
		if route == "" {
			route = "unmatched"
		}
		level := slog.LevelInfo
		if status >= http.StatusInternalServerError {
			level = slog.LevelError
		}
		s.logger.LogAttrs(r.Context(), level, "api.request",
			slog.String("method", r.Method),
			slog.String("route", route),
			slog.Int("status", status),
			slog.Int("bytes", response.bytes),
			slog.Int64("duration_ms", time.Since(started).Milliseconds()),
		)
	})
}

type loggingResponseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *loggingResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *loggingResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(body)
	w.bytes += n
	return n, err
}

func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) Address(ctx context.Context) (string, error) {
	cfg, err := s.load(ctx)
	return cfg.API.Address, err
}

func (s *Server) SetBoundAddress(address string) {
	s.boundLAN = !isLoopbackAddress(address)
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if publicDocumentationPath(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		cfg, err := s.load(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "configuration unavailable")
			return
		}
		if s.boundLAN || cfg.API.AllowLAN {
			if cfg.API.PairingToken == "" {
				writeError(w, http.StatusInternalServerError, "authentication unavailable")
				return
			}
			provided := ""
			const prefix = "Bearer "
			if value := r.Header.Get("Authorization"); len(value) > len(prefix) && value[:len(prefix)] == prefix {
				provided = value[len(prefix):]
			}
			if subtle.ConstantTimeCompare([]byte(provided), []byte(cfg.API.PairingToken)) != 1 {
				writeError(w, http.StatusUnauthorized, "authentication required")
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func publicDocumentationPath(path string) bool {
	return path == "/docs" || path == "/scalar.js" || path == "/api/v1/openapi.json"
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", "GET")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.modelReloader == nil {
		writeError(w, http.StatusServiceUnavailable, "model reload unavailable")
		return
	}
	snapshot, err := s.modelReloader.Reload(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "model reload failed")
		return
	}
	snapshot = publicModelSnapshot(snapshot)
	errors := make([]string, 0)
	for _, profile := range snapshot.Profiles {
		if profile.Error != "" {
			errors = append(errors, profile.Error)
		}
	}
	writeJSON(w, http.StatusOK, struct {
		Profiles []models.Profile               `json:"profiles"`
		Active   map[models.Role]models.Profile `json:"active"`
		Errors   []string                       `json:"errors"`
	}{snapshot.Profiles, snapshot.Active, errors})
}

func publicModelSnapshot(snapshot models.Snapshot) models.Snapshot {
	profiles := make([]models.Profile, 0, len(snapshot.Profiles))
	for _, profile := range snapshot.Profiles {
		if profile.Role != models.RoleIntent {
			profiles = append(profiles, profile)
		}
	}
	active := make(map[models.Role]models.Profile, len(snapshot.Active))
	for role, profile := range snapshot.Active {
		if role != models.RoleIntent {
			active[role] = profile
		}
	}
	snapshot.Profiles = profiles
	snapshot.Active = active
	return snapshot
}

func serveEmbedded(contentType string, content []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", "GET")
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		w.Header().Set("Content-Type", contentType)
		_, _ = w.Write(content)
	}
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		cfg, err := s.load(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "configuration unavailable")
			return
		}
		writeJSON(w, http.StatusOK, publicConfig(cfg))
	case http.MethodPut:
		s.updateConfig(w, r)
	default:
		w.Header().Set("Allow", "GET, PUT")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

type configUpdate struct {
	HomeAssistant *struct {
		URL            *string `json:"url"`
		Token          *string `json:"token"`
		TimeoutSeconds *int64  `json:"timeout_seconds"`
	} `json:"home_assistant"`
	API *struct {
		Address      *string `json:"address"`
		AllowLAN     *bool   `json:"allow_lan"`
		PairingToken *string `json:"pairing_token"`
	} `json:"api"`
}

func (s *Server) updateConfig(w http.ResponseWriter, r *http.Request) {
	var update configUpdate
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, "invalid configuration")
		return
	}
	if update.API != nil && update.API.PairingToken != nil && *update.API.PairingToken != Redacted {
		writeError(w, http.StatusBadRequest, "pairing token can only be changed via POST /api/v1/config/pairing-token")
		return
	}
	refresh := false
	invalid := false
	cfg, err := s.updateStoredConfig(r.Context(), func(cfg *config.Config) error {
		if update.HomeAssistant != nil {
			refresh = true
			if update.HomeAssistant.URL != nil {
				cfg.HomeAssistant.URL = *update.HomeAssistant.URL
			}
			if update.HomeAssistant.Token != nil && *update.HomeAssistant.Token != Redacted {
				cfg.HomeAssistant.Token = *update.HomeAssistant.Token
			}
			if update.HomeAssistant.TimeoutSeconds != nil {
				cfg.HomeAssistant.Timeout = time.Duration(*update.HomeAssistant.TimeoutSeconds) * time.Second
			}
		}
		if update.API != nil {
			if update.API.Address != nil {
				cfg.API.Address = *update.API.Address
			}
			if update.API.AllowLAN != nil {
				cfg.API.AllowLAN = *update.API.AllowLAN
			}
			if s.boundLAN && (!cfg.API.AllowLAN || isLoopbackAddress(cfg.API.Address)) {
				invalid = true
				return errors.New("restart required for LAN API binding changes")
			}
		}
		if err := cfg.Validate(); err != nil {
			invalid = true
			return err
		}
		return nil
	})
	if err != nil {
		if invalid {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if errors.Is(err, errConfigUnavailable) {
			writeError(w, http.StatusInternalServerError, "configuration unavailable")
			return
		}
		writeError(w, http.StatusInternalServerError, "configuration write failed")
		return
	}
	if refresh && s.refresher != nil {
		if _, err := s.refresher.Discover(r.Context()); err != nil {
			writeError(w, http.StatusBadGateway, "provider refresh failed")
			return
		}
	}
	writeJSON(w, http.StatusOK, publicConfig(cfg))
}

func isLoopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	return host == "localhost" || net.ParseIP(host).IsLoopback()
}

func (s *Server) handlePairingToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		writeError(w, http.StatusInternalServerError, "pairing token generation failed")
		return
	}
	token := base64.RawURLEncoding.EncodeToString(bytes)
	if _, err := s.updateStoredConfig(r.Context(), func(cfg *config.Config) error {
		cfg.API.PairingToken = token
		return nil
	}); err != nil {
		if errors.Is(err, errConfigUnavailable) {
			writeError(w, http.StatusInternalServerError, "configuration unavailable")
			return
		}
		writeError(w, http.StatusInternalServerError, "configuration write failed")
		return
	}
	writeJSON(w, http.StatusCreated, map[string]string{"pairing_token": token})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	if s.refresher == nil {
		writeError(w, http.StatusServiceUnavailable, "provider refresh unavailable")
		return
	}
	if _, err := s.refresher.Discover(r.Context()); err != nil {
		writeError(w, http.StatusBadGateway, "provider refresh failed")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) load(ctx context.Context) (config.Config, error) {
	var cfg config.Config
	err := s.config.Load(ctx, &cfg)
	return cfg, err
}

func (s *Server) updateStoredConfig(ctx context.Context, update func(*config.Config) error) (config.Config, error) {
	s.configMu.Lock()
	defer s.configMu.Unlock()
	cfg, err := s.load(ctx)
	if err != nil {
		return config.Config{}, errors.Join(errConfigUnavailable, err)
	}
	if err := update(&cfg); err != nil {
		return config.Config{}, err
	}
	if err := s.config.Save(ctx, cfg); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

func publicConfig(cfg config.Config) any {
	secret := func(value string) string {
		if value != "" {
			return Redacted
		}
		return ""
	}
	return struct {
		HomeAssistant struct {
			URL            string `json:"url"`
			Token          string `json:"token"`
			TimeoutSeconds int64  `json:"timeout_seconds"`
		} `json:"home_assistant"`
		API struct {
			Address      string `json:"address"`
			AllowLAN     bool   `json:"allow_lan"`
			PairingToken string `json:"pairing_token"`
		} `json:"api"`
	}{
		HomeAssistant: struct {
			URL            string `json:"url"`
			Token          string `json:"token"`
			TimeoutSeconds int64  `json:"timeout_seconds"`
		}{cfg.HomeAssistant.URL, secret(cfg.HomeAssistant.Token), int64(cfg.HomeAssistant.Timeout / time.Second)},
		API: struct {
			Address      string `json:"address"`
			AllowLAN     bool   `json:"allow_lan"`
			PairingToken string `json:"pairing_token"`
		}{cfg.API.Address, cfg.API.AllowLAN, secret(cfg.API.PairingToken)},
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
