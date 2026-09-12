package api

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"time"

	"peanut/internal/config"
	"peanut/internal/intent"
	"peanut/internal/storage/sqlite"
)

const Redacted = "[REDACTED]"

type Refresher interface {
	Discover(context.Context) ([]intent.Capability, error)
}

type Server struct {
	config      *sqlite.ConfigStore
	refresher   Refresher
	handler     http.Handler
	requireAuth bool
}

func NewServer(db *sqlite.DB, refresher Refresher) *Server {
	server := &Server{config: sqlite.NewConfigStore(db), refresher: refresher}
	if cfg, err := server.load(context.Background()); err == nil {
		server.requireAuth = cfg.API.AllowLAN
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/config", server.handleConfig)
	mux.HandleFunc("/api/v1/config/pairing-token", server.handlePairingToken)
	mux.HandleFunc("/api/v1/config/refresh", server.handleRefresh)
	server.handler = server.authenticate(mux)
	return server
}

func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) Address(ctx context.Context) (string, error) {
	cfg, err := s.load(ctx)
	return cfg.API.Address, err
}

func (s *Server) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cfg, err := s.load(r.Context())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "configuration unavailable")
			return
		}
		if s.requireAuth {
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
		Address  *string `json:"address"`
		AllowLAN *bool   `json:"allow_lan"`
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
	cfg, err := s.load(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "configuration unavailable")
		return
	}
	refresh := false
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
	}
	if err := cfg.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.config.Save(r.Context(), cfg); err != nil {
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
	cfg, err := s.load(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "configuration unavailable")
		return
	}
	cfg.API.PairingToken = token
	if err := s.config.Save(r.Context(), cfg); err != nil {
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
