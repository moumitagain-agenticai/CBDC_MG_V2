//go:build integration

package integration

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/fineract/cbdc/india-connector/internal/adapters/repository"
	"github.com/fineract/cbdc/india-connector/internal/config"
)

// TestMigrations exercises Migrate + Rollback against a real Postgres. It is
// skipped when DATABASE_DSN is not set, so the suite stays green without a DB.
func TestMigrations(t *testing.T) {
	dsn := os.Getenv("DATABASE_DSN")
	if dsn == "" {
		t.Skip("DATABASE_DSN not set; skipping DB migration test")
	}

	db, err := repository.OpenDB(config.DatabaseConfig{
		DSN: dsn, MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: time.Minute,
	})
	require.NoError(t, err)
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	const existsQ = `SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = 'cbdc_transactions')`

	require.NoError(t, repository.Migrate(ctx, db, zap.NewNop()))

	var exists bool
	require.NoError(t, db.QueryRowContext(ctx, existsQ).Scan(&exists))
	require.True(t, exists, "table should exist after Migrate")

	require.NoError(t, repository.Rollback(ctx, db, zap.NewNop(), 1))
	require.NoError(t, db.QueryRowContext(ctx, existsQ).Scan(&exists))
	require.False(t, exists, "table should be gone after Rollback")

	// Restore a clean, migrated state.
	require.NoError(t, repository.Migrate(ctx, db, zap.NewNop()))
}
