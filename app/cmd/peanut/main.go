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
	runtimeConfig, err := config.Load(databasePath)
	if err != nil {
		return err
	}
	if len(args) <= 1 {
		return usage()
	}
	args = args[1:]
	if dependencies.ModelRoot == "" {
		dependencies.ModelRoot = runtimeConfig.Models.Root
	}
	if args[0] == "run" && dependencies.Run == nil {
		dependencies.Run = func(runCtx context.Context) error {
			return runConfigured(runCtx, runtimeConfig, db)
		}
	}
	return dispatch(ctx, args, dependencies, output)
}

func serveAPI(ctx context.Context, db *sqlite.DB) error {
	provider := providers.NewHomeAssistantProvider(db, nil)
	var cfg config.Config
	if err := sqlite.NewConfigStore(db).Load(ctx, &cfg); err != nil {
		return err
	}
	modelOwner := newReloadableModels(buildModelSet(cfg))
	modelRegistry := newRuntimeModelRegistry(cfg, db, modelOwner)
	server := api.NewServer(db, provider, modelRegistry)
	address, err := server.Address(ctx)
	if err != nil {
		return err
	}
	server.SetBoundAddress(address)
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
