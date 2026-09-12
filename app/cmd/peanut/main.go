package main

import (
	"context"
	"log"

	"peanut/internal/config"
	"peanut/internal/storage/sqlite"
)

func main() {
	ctx := context.Background()
	db, err := sqlite.Open(ctx, config.DefaultDatabasePath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		log.Fatal(err)
	}
	if err := config.PersistDefaults(ctx, db); err != nil {
		log.Fatal(err)
	}
	if _, err := config.Load(config.DefaultDatabasePath); err != nil {
		log.Fatal(err)
	}
}
