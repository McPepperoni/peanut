package sqlite

import (
	"context"
	"path/filepath"
	"testing"
)

func TestMigrateCreatesTables(t *testing.T) {
	ctx := context.Background()
	db, err := Open(ctx, filepath.Join(t.TempDir(), "peanut.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	for _, table := range []string{"schema_migrations", "settings", "speakers", "providers", "devices", "capabilities"} {
		var found string
		err := db.QueryRowContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&found)
		if err != nil {
			t.Errorf("table %q: %v", table, err)
		}
	}
}

func TestDataPersistsAfterReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "peanut.db")

	db, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ('language', 'en')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var value string
	if err := db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'language'`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != "en" {
		t.Fatalf("value after reopen = %q, want en", value)
	}
}

func TestOpenCreatesDatabaseDirectory(t *testing.T) {
	db, err := Open(context.Background(), filepath.Join(t.TempDir(), "data", "peanut.db"))
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
}
