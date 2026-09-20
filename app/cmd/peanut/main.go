package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"peanut/internal/api"
	"peanut/internal/audio/playback"
	"peanut/internal/config"
	"peanut/internal/logging"
	"peanut/internal/models"
	"peanut/internal/providers"
	"peanut/internal/storage/sqlite"
)

func main() {
	logger := logging.New(os.Stderr)
	ctx, stop := signal.NotifyContext(context.Background(), terminationSignals()...)
	defer stop()
	if err := runMain(ctx, os.Args, config.DefaultDatabasePath, commandDependencies{Logger: logger}, os.Stdout); err != nil {
		logger.Error("process.exit", "component", "process", "status", "failed", "error_type", "operation_failed")
		os.Exit(1)
	}
}

func terminationSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM}
}

func runMain(ctx context.Context, args []string, databasePath string, dependencies commandDependencies, output io.Writer) error {
	dependencies.Logger = logging.Normalize(dependencies.Logger)
	dependencies.Logger.Info("process.start", "component", "process", "status", "started")
	db, err := sqlite.Open(ctx, databasePath)
	if err != nil {
		return err
	}
	defer db.Close()
	dependencies.Logger.Info("database.open", "component", "database", "status", "succeeded")
	if err := db.Migrate(ctx); err != nil {
		return err
	}
	dependencies.Logger.Info("database.migrate", "component", "database", "status", "succeeded")
	if err := config.PersistDefaults(ctx, db); err != nil {
		return err
	}
	dependencies.Logger.Info("database.defaults", "component", "database", "status", "succeeded")
	if len(args) > 1 && args[1] == "api" {
		return serveAPIWithLogger(ctx, db, dependencies.Logger)
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
			return runConfiguredWithLogger(runCtx, runtimeConfig, db, dependencies.Logger)
		}
	}
	if args[0] == "test-audio" && dependencies.Player == nil {
		dependencies.Player = playback.SystemPlayer{Device: runtimeConfig.Audio.OutputDevice}
	}
	var commandRuntime *configuredRuntime
	if needsCommandRuntime(args[0], dependencies) {
		commandRuntime, err = newCommandRuntimeWithLogger(ctx, runtimeConfig, db, args[0], dependencies.Logger)
		if err != nil {
			return err
		}
		defer func() {
			if cleanupErr := errors.Join(closeRuntimeResources(commandRuntime.capture, commandRuntime.player), commandRuntime.modelSet.Close()); cleanupErr != nil {
				dependencies.Logger.Error("runtime.cleanup", "component", "runtime", "status", "failed", "error_type", "operation_failed")
			}
		}()
		configured := commandRuntime.commandDependencies(ctx)
		configured.Logger = dependencies.Logger
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
	err = dispatch(ctx, args, dependencies, output)
	if err != nil {
		dependencies.Logger.Error("dispatch.complete", "component", "dispatch", "status", "failed", "error_type", "operation_failed")
		return err
	}
	dependencies.Logger.Info("dispatch.complete", "component", "dispatch", "status", "succeeded")
	return nil
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
	return serveAPIWithLogger(ctx, db, logging.Nop())
}

func serveAPIWithLogger(ctx context.Context, db *sqlite.DB, logger *slog.Logger) (err error) {
	logger = logging.Normalize(logger)
	logger.Info("api.start", "component", "api", "status", "starting")
	defer func() {
		if err != nil {
			logger.Error("api.stop", "component", "api", "status", "failed", "error_type", "operation_failed")
		}
	}()
	provider := providers.NewHomeAssistantProvider(db, nil)
	var cfg config.Config
	if err := sqlite.NewConfigStore(db).Load(ctx, &cfg); err != nil {
		return err
	}
	modelRegistry := newAPIModelRegistry(cfg, db)
	server := api.NewServerWithLogger(db, provider, modelRegistry, logger)
	address, err := server.Address(ctx)
	if err != nil {
		return err
	}
	server.SetBoundAddress(address)
	httpServer := &http.Server{Addr: address, Handler: server.Handler()}
	err = runHTTPServer(ctx, httpServer.ListenAndServe, func() error {
		return shutdownHTTPServer(httpServer, gracefulHTTPShutdownTimeout)
	})
	if err != nil {
		return err
	}
	logger.Info("api.stop", "component", "api", "status", "stopped")
	return nil
}

func newAPIModelRegistry(cfg config.Config, db *sqlite.DB) *models.Registry {
	return models.NewRegistry(cfg.Models.Root, sqlite.NewModelStore(db), nil)
}
