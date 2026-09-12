package sqlite

import (
	"context"
	_ "embed"
	"fmt"
)

//go:embed migrations/001_initial.sql
var initialMigration string

func (db *DB) Migrate(ctx context.Context) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, initialMigration); err != nil {
		return fmt.Errorf("apply migration: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration: %w", err)
	}
	return nil
}
