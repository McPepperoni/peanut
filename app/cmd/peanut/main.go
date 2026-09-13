package main

import (
	"context"
	"io"
	"log"
	"os"

	"peanut/internal/config"
	"peanut/internal/storage/sqlite"
)

func main() {
	if err := runMain(context.Background(), os.Args, config.DefaultDatabasePath, commandDependencies{}, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func runMain(ctx context.Context, args []string, databasePath string, dependencies commandDependencies, output io.Writer) error {
	db, err := sqlite.Open(ctx, databasePath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		return err
	}
	if err := config.PersistDefaults(ctx, db); err != nil {
		return err
	}
	if _, err := config.Load(databasePath); err != nil {
		return err
	}
	if len(args) == 0 {
		return usage()
	}
	if len(args) > 0 {
		args = args[1:]
	}
	return dispatch(ctx, args, dependencies, output)
}
