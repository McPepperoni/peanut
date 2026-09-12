package sqlite

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"peanut/internal/intent"
)

const runtimeConfigKey = "runtime_config"

type ConfigStore struct{ db *DB }

func NewConfigStore(db *DB) *ConfigStore { return &ConfigStore{db: db} }

func (s *ConfigStore) Load(ctx context.Context, destination any) error {
	if s == nil || s.db == nil || destination == nil {
		return errors.New("config store and destination are required")
	}
	var value []byte
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, runtimeConfigKey).Scan(&value); err != nil {
		return fmt.Errorf("load runtime config: %w", err)
	}
	if err := json.Unmarshal(value, destination); err != nil {
		return fmt.Errorf("decode runtime config: %w", err)
	}
	return nil
}

func (s *ConfigStore) Save(ctx context.Context, value any) error {
	if s == nil || s.db == nil {
		return errors.New("config store is required")
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode runtime config: %w", err)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin runtime config write: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, runtimeConfigKey, encoded); err != nil {
		return fmt.Errorf("write runtime config: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit runtime config: %w", err)
	}
	return nil
}

type HomeAssistantConfig struct {
	URL     string
	Token   string
	Timeout time.Duration
}

func (s *ConfigStore) HomeAssistant(ctx context.Context) (HomeAssistantConfig, error) {
	var value struct{ HomeAssistant HomeAssistantConfig }
	if err := s.Load(ctx, &value); err != nil {
		return HomeAssistantConfig{}, err
	}
	if value.HomeAssistant.URL == "" {
		value.HomeAssistant.URL = "http://127.0.0.1:8123"
	}
	if value.HomeAssistant.Timeout <= 0 {
		value.HomeAssistant.Timeout = 10 * time.Second
	}
	return value.HomeAssistant, nil
}

type CapabilityStore struct{ db *DB }

func NewCapabilityStore(db *DB) *CapabilityStore { return &CapabilityStore{db: db} }

func (s *CapabilityStore) Replace(ctx context.Context, providerID, providerType string, capabilities []intent.Capability) error {
	if s == nil || s.db == nil || providerID == "" || providerType == "" {
		return errors.New("capability store, provider ID, and provider type are required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin capability refresh: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO providers (id, type) VALUES (?, ?) ON CONFLICT(id) DO UPDATE SET type = excluded.type`, providerID, providerType); err != nil {
		return fmt.Errorf("store provider: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM devices WHERE provider_id = ?`, providerID); err != nil {
		return fmt.Errorf("clear provider devices: %w", err)
	}
	for _, capability := range capabilities {
		if capability.ProviderID != providerID {
			return fmt.Errorf("capability %q belongs to provider %q", capability.ID, capability.ProviderID)
		}
		actions, err := json.Marshal(capability.Actions)
		if err != nil {
			return fmt.Errorf("encode capability %q: %w", capability.ID, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO devices (id, provider_id, name, room) VALUES (?, ?, ?, ?) ON CONFLICT(id) DO UPDATE SET provider_id = excluded.provider_id, name = excluded.name, room = excluded.room`, capability.DeviceID, providerID, capability.Name, capability.Room); err != nil {
			return fmt.Errorf("store device %q: %w", capability.DeviceID, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO capabilities (id, device_id, type, actions, refreshed_at) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`, capability.ID, capability.DeviceID, capability.Type, actions); err != nil {
			return fmt.Errorf("store capability %q: %w", capability.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit capability refresh: %w", err)
	}
	return nil
}

func (s *CapabilityStore) List(ctx context.Context, providerID string) ([]intent.Capability, error) {
	if s == nil || s.db == nil || providerID == "" {
		return nil, errors.New("capability store and provider ID are required")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT c.id, d.provider_id, c.device_id, c.type, d.name, d.room, c.actions FROM capabilities c JOIN devices d ON d.id = c.device_id WHERE d.provider_id = ? ORDER BY c.id`, providerID)
	if err != nil {
		return nil, fmt.Errorf("load capabilities: %w", err)
	}
	defer rows.Close()
	var capabilities []intent.Capability
	for rows.Next() {
		var capability intent.Capability
		var actions []byte
		if err := rows.Scan(&capability.ID, &capability.ProviderID, &capability.DeviceID, &capability.Type, &capability.Name, &capability.Room, &actions); err != nil {
			return nil, fmt.Errorf("scan capability: %w", err)
		}
		if err := json.Unmarshal(actions, &capability.Actions); err != nil {
			return nil, fmt.Errorf("decode capability %q: %w", capability.ID, err)
		}
		capabilities = append(capabilities, capability)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load capabilities: %w", err)
	}
	return capabilities, nil
}
