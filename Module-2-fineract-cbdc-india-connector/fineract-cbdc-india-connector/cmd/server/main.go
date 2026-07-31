package main

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

	"github.com/fineract/cbdc/india-connector/internal/adapters/api"
	"github.com/fineract/cbdc/india-connector/internal/adapters/client"
	"github.com/fineract/cbdc/india-connector/internal/adapters/repository"
	"github.com/fineract/cbdc/india-connector/internal/config"
	"github.com/fineract/cbdc/india-connector/internal/ports"
	"github.com/fineract/cbdc/india-connector/internal/service"
	"github.com/fineract/cbdc/india-connector/pkg/logger"
	"github.com/fineract/cbdc/india-connector/pkg/metrics"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fatal:", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "configs/config.yaml", "path to YAML config file")
	rollback := flag.Int("rollback", 0, "roll back the last N database migrations and exit")
	flag.Parse()

	// Load .env if present (local/dev convenience); ignore if absent.
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

	cbdcClient, err := client.New(cfg.CBDC)
	if err != nil {
		return fmt.Errorf("init cbdc client: %w", err)
	}

	// Persistence is optional. Keep repo a true nil interface when disabled so
	// service nil-checks work correctly.
	var repo ports.TransactionRepository
	if cfg.Database.Enabled {
		db, err := repository.OpenDB(cfg.Database)
		if err != nil {
			return fmt.Errorf("open database: %w", err)
		}
		defer func() { _ = db.Close() }()

		// Rollback-and-exit mode.
		if *rollback > 0 {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			if err := repository.Rollback(ctx, db, log, *rollback); err != nil {
				return fmt.Errorf("rollback: %w", err)
			}
			log.Info("rollback complete", zap.Int("count", *rollback))
			return nil
		}

		// Auto-apply pending migrations on startup.
		mctx, mcancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer mcancel()
		if err := repository.Migrate(mctx, db, log); err != nil {
			return fmt.Errorf("run migrations: %w", err)
		}

		repo = repository.New(db)
		log.Info("persistence enabled")
	} else {
		if *rollback > 0 {
			return fmt.Errorf("-rollback requires a database (set DATABASE_DSN)")
		}
		log.Warn("persistence disabled; transactions will not be recorded")
	}

	svc := service.NewConnector(cbdcClient, repo, m, log)
	handler := api.NewHandler(svc)
	router := api.NewRouter(handler, m, log, cfg.Server.RateLimitPerMin)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:      router,
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}

	// Start server.
	serverErr := make(chan error, 1)
	go func() {
		log.Info("server listening", zap.Int("port", cfg.Server.Port))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
	}()

	// Wait for shutdown signal or fatal server error.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErr:
		return fmt.Errorf("server error: %w", err)
	case sig := <-stop:
		log.Info("shutdown signal received", zap.String("signal", sig.String()))
	}

	// Graceful shutdown: drain in-flight requests.
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("graceful shutdown failed: %w", err)
	}
	log.Info("server stopped cleanly")
	return nil
}
