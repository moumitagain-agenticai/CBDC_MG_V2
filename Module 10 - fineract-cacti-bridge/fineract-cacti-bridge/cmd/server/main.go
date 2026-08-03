package main

import "github.com/fineract/cacti-bridge/pkg/flog"
import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"go.uber.org/zap"

	"github.com/fineract/cacti-bridge/internal/adapters/api"
	"github.com/fineract/cacti-bridge/internal/adapters/client"
	"github.com/fineract/cacti-bridge/internal/adapters/repository"
	"github.com/fineract/cacti-bridge/internal/config"
	"github.com/fineract/cacti-bridge/internal/ports"
	"github.com/fineract/cacti-bridge/internal/service"
	"github.com/fineract/cacti-bridge/pkg/logger"
	"github.com/fineract/cacti-bridge/pkg/metrics"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "configs/config.yaml", "path to YAML config file")
	migrateRollback := flag.Int("migrate-rollback", 0, "roll back the last N database migrations and exit")
	rollbackID := flag.String("rollback", "", "compensate/roll back a specific settlement by id and exit")
	flag.Parse()

	_ = godotenv.Load()

	cfg, err := config.Load(*configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log, err := logger.New(cfg.Log.Level, cfg.Log.JSON)
	if err != nil {
		return fmt.Errorf("init logger: %w", err)
	}
	defer func() { _ = log.Sync() }()

	m := metrics.New()

	source, err := client.New(cfg.Ledgers.Source)
	if err != nil {
		return fmt.Errorf("init source connector: %w", err)
	}
	dest, err := client.New(cfg.Ledgers.Dest)
	if err != nil {
		return fmt.Errorf("init dest connector: %w", err)
	}

	// Select the repository: durable Postgres when configured, else in-memory.
	var repo ports.SettlementRepository
	if cfg.Database.Enabled {
		db, err := repository.OpenDB(cfg.Database)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer func() { _ = db.Close() }()

		if *migrateRollback > 0 {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := repository.Rollback(ctx, db, log, *migrateRollback); err != nil {
				return fmt.Errorf("migrate rollback: %w", err)
			}
			log.Info("migration rollback complete", zap.Int("count", *migrateRollback))
			return nil
		}

		mctx, mcancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer mcancel()
		if err := repository.Migrate(mctx, db, log); err != nil {
			return fmt.Errorf("run migrations: %w", err)
		}
		repo = repository.New(db)
		log.Info("durable storage enabled (postgres)")
	} else {
		if *migrateRollback > 0 {
			return fmt.Errorf("-migrate-rollback requires a database (set DATABASE_DSN)")
		}
		repo = repository.NewMemory()
		log.Warn("using in-memory storage; settlements are not durable across restarts")
	}

	coord := service.NewCoordinator(source, dest, repo, cfg.Settlement, m, log)

	// One-shot: compensate a specific settlement and exit.
	if *rollbackID != "" {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.Settlement.StepTimeout*3)
		defer cancel()
		t, err := coord.Rollback(ctx, *rollbackID)
		if err != nil {
			return fmt.Errorf("rollback %s: %w", *rollbackID, err)
		}
		log.Info("rollback finished", zap.String("id", t.ID), zap.String("status", string(t.Status)))
		return nil
	}

	// Recover any in-flight settlements left by a previous crash.
	if cfg.Settlement.RecoverOnStartup {
		rctx, rcancel := context.WithTimeout(context.Background(), 60*time.Second)
		if n, err := coord.Recover(rctx); err != nil {
			log.Error("startup recovery encountered errors", zap.Error(err))
		} else if n > 0 {
			log.Info("startup recovery complete", zap.Int("recovered", n))
		}
		rcancel()
	}

	handler := api.NewHandler(coord, 1<<20)
	router := api.NewRouter(handler, m, log, cfg.Server.RateLimitPerMin)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	serverErr := make(chan error, 1)
	go func() {
		log.Info("server listening", zap.Int("port", cfg.Server.Port))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	case sig := <-stop:
		log.Info("shutdown signal received", zap.String("signal", sig.String()))
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}
	log.Info("server stopped cleanly")
	return nil
}

// flogMarker registers this source file with the Logrus per-file logger,
// producing logs/10_main.log at runtime.
var _ = func() bool { flog.For("10_main").Info("source file initialized"); return true }()
