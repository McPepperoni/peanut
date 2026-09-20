package sqlite

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"peanut/internal/models"
)

type ModelStore struct{ db *DB }

func NewModelStore(db *DB) *ModelStore { return &ModelStore{db: db} }

func (s *ModelStore) ReplaceSnapshot(ctx context.Context, profiles []models.Profile) error {
	if s == nil || s.db == nil {
		return errors.New("model store is required")
	}
	profiles = append([]models.Profile(nil), profiles...)
	for index := range profiles {
		profile := &profiles[index]
		if profile.Path == "" || unsafeRelativePath(profile.Path) {
			return fmt.Errorf("model %q path must be relative", profile.ID)
		}
		if unsafeRelativePath(profile.Entry) {
			if profile.Valid {
				return fmt.Errorf("model %q entry must be relative without traversal", profile.ID)
			}
			profile.Entry = ""
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin model snapshot: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM models`); err != nil {
		return fmt.Errorf("clear model snapshot: %w", err)
	}
	explicitActive := make(map[models.Role]bool)
	for _, profile := range profiles {
		explicitActive[profile.Role] = explicitActive[profile.Role] || profile.Active
	}
	active := make(map[models.Role]bool)
	for _, profile := range profiles {
		isActive := profile.Active || (!explicitActive[profile.Role] && profile.Valid && !active[profile.Role])
		if isActive && active[profile.Role] {
			return fmt.Errorf("multiple active models for role %q", profile.Role)
		}
		active[profile.Role] = active[profile.Role] || isActive
		if _, err := tx.ExecContext(ctx, `INSERT INTO models (id, role, runtime, path, entry, sha256, valid, error, active, refreshed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP)`, profile.ID, profile.Role, profile.Runtime, profile.Path, profile.Entry, profile.SHA256, profile.Valid, profile.Error, isActive); err != nil {
			return fmt.Errorf("store model %q: %w", profile.ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit model snapshot: %w", err)
	}
	return nil
}

func unsafeRelativePath(path string) bool {
	if path == "" {
		return false
	}
	if filepath.IsAbs(filepath.FromSlash(path)) {
		return true
	}
	for _, part := range strings.FieldsFunc(path, func(character rune) bool { return character == '/' || character == '\\' }) {
		if part == ".." {
			return true
		}
	}
	return false
}

func (s *ModelStore) List(ctx context.Context) ([]models.Profile, error) {
	if s == nil || s.db == nil {
		return nil, errors.New("model store is required")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, role, runtime, path, entry, sha256, valid, error, active FROM models ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	defer rows.Close()
	var profiles []models.Profile
	for rows.Next() {
		var profile models.Profile
		if err := rows.Scan(&profile.ID, &profile.Role, &profile.Runtime, &profile.Path, &profile.Entry, &profile.SHA256, &profile.Valid, &profile.Error, &profile.Active); err != nil {
			return nil, fmt.Errorf("scan model: %w", err)
		}
		profiles = append(profiles, profile)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list models: %w", err)
	}
	return profiles, nil
}
