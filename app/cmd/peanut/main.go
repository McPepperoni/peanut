package main

import (
	"context"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"peanut/internal/api"
	"peanut/internal/audio/playback"
	"peanut/internal/config"
	"peanut/internal/models"
	"peanut/internal/providers"
	"peanut/internal/storage/sqlite"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), terminationSignals()...)
	defer stop()
	if err := runMain(ctx, os.Args, config.DefaultDatabasePath, commandDependencies{}, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

func terminationSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
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
	if args[0] == "test-audio" && dependencies.Player == nil {
		dependencies.Player = playback.SystemPlayer{Device: runtimeConfig.Audio.OutputDevice}
	}
	var commandRuntime *configuredRuntime
	if needsCommandRuntime(args[0], dependencies) {
		commandRuntime, err = newCommandRuntime(ctx, runtimeConfig, db, args[0])
		if err != nil {
			return err
		}
		defer func() {
			_ = closeRuntimeResources(commandRuntime.capture, commandRuntime.player)
			_ = commandRuntime.modelSet.Close()
		}()
		configured := commandRuntime.commandDependencies(ctx)
		if dependencies.Player == nil {
			dependencies.Player = configured.Player
		}
		if dependencies.Synthesizer == nil {
			dependencies.Synthesizer = configured.Synthesizer
		}
		if dependencies.Transcriber == nil {
			dependencies.Transcriber = configured.Transcriber
		}
		if dependencies.Speaker == nil {
			dependencies.Speaker = configured.Speaker
		}
		if dependencies.Enroll == nil {
			dependencies.Enroll = configured.Enroll
		}
	}
	return dispatch(ctx, args, dependencies, output)
}

func needsCommandRuntime(command string, dependencies commandDependencies) bool {
	switch command {
	case "speak":
		return dependencies.Player == nil || dependencies.Synthesizer == nil
	case "transcribe":
		return dependencies.Transcriber == nil
	case "enroll":
		return dependencies.Enroll == nil
	case "test-audio":
		return false
	default:
		return false
	}
}

func serveAPI(ctx context.Context, db *sqlite.DB) error {
	provider := providers.NewHomeAssistantProvider(db, nil)
	var cfg config.Config
	if err := sqlite.NewConfigStore(db).Load(ctx, &cfg); err != nil {
		return err
	}
	modelRegistry := newAPIModelRegistry(cfg, db)
	server := api.NewServer(db, provider, modelRegistry)
	address, err := server.Address(ctx)
	if err != nil {
		return err
	}
	server.SetBoundAddress(address)
	httpServer := &http.Server{Addr: address, Handler: server.Handler()}
	return runHTTPServer(ctx, httpServer.ListenAndServe, func() error {
		return shutdownHTTPServer(httpServer, gracefulHTTPShutdownTimeout)
	})
}

func newAPIModelRegistry(cfg config.Config, db *sqlite.DB) *models.Registry {
	return models.NewRegistry(cfg.Models.Root, sqlite.NewModelStore(db), nil)
}
