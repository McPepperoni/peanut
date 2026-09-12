package main

import (
	"context"
	"log"

	"peanut/internal/config"
	"peanut/internal/storage/sqlite"
)

func main() {
	ctx := context.Background()
	cfg, err := config.Load(config.DefaultDatabasePath)
	if err != nil {
		log.Fatal(err)
	}
	db, err := sqlite.Open(ctx, cfg.DatabasePath)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		log.Fatal(err)
	}
}
