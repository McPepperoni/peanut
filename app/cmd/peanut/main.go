package main

import (
	"context"
	"errors"
	"io"
	"log"
	"net/http"
	"os"

	"peanut/internal/api"
	"peanut/internal/config"
	"peanut/internal/providers"
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
	if len(args) > 1 && args[1] == "api" {
		return serveAPI(ctx, db)
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

func serveAPI(ctx context.Context, db *sqlite.DB) error {
	provider := providers.NewHomeAssistantProvider(db, nil)
	server := api.NewServer(db, provider)
	address, err := server.Address(ctx)
	if err != nil {
		return err
	}
	httpServer := &http.Server{Addr: address, Handler: server.Handler()}
	go func() {
		<-ctx.Done()
		_ = httpServer.Shutdown(context.Background())
	}()
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
