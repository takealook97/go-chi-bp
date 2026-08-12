// Package postgrestest provides isolated PostgreSQL databases for integration tests.
package postgrestest

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Database is an isolated schema with all repository migrations applied by Goose.
type Database struct {
	Pool       *pgxpool.Pool
	provider   *goose.Provider
	ctx        context.Context
	rolledBack bool
}

// New creates an isolated schema and applies the repository's real Goose migrations.
func New(t *testing.T) *Database {
	t.Helper()

	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	schema := "integration_test_" + randomHex(t, 8)
	quotedSchema := pgx.Identifier{schema}.Sanitize()

	basePool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		cancel()
		t.Fatalf("connect to integration database: %v", err)
	}
	if _, err := basePool.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		basePool.Close()
		cancel()
		t.Fatalf("create isolated test schema: %v", err)
	}

	database := &Database{ctx: ctx}
	var migrationDatabase *sql.DB
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cleanupCancel()
		if database.provider != nil && !database.rolledBack {
			if _, rollbackErr := database.provider.DownTo(cleanupCtx, 0); rollbackErr != nil {
				t.Errorf("roll back migrations: %v", rollbackErr)
			}
		}
		if database.Pool != nil {
			database.Pool.Close()
		}
		if migrationDatabase != nil {
			if closeErr := migrationDatabase.Close(); closeErr != nil {
				t.Errorf("close migration database: %v", closeErr)
			}
		}
		if _, cleanupErr := basePool.Exec(cleanupCtx, "DROP SCHEMA "+quotedSchema+" CASCADE"); cleanupErr != nil {
			t.Errorf("drop isolated test schema: %v", cleanupErr)
		}
		basePool.Close()
		cancel()
	})

	connConfig, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse integration database configuration: %v", err)
	}
	connConfig.RuntimeParams["search_path"] = schema
	migrationDatabase = stdlib.OpenDB(*connConfig)

	provider, err := goose.NewProvider(goose.DialectPostgres, migrationDatabase, migrationFiles(t))
	if err != nil {
		t.Fatalf("create migration provider: %v", err)
	}
	database.provider = provider
	if _, err := provider.Up(ctx); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse isolated pool configuration: %v", err)
	}
	poolConfig.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatalf("connect to isolated test schema: %v", err)
	}
	database.Pool = pool

	return database
}

// Rollback rolls every migration back through Goose for down-migration verification.
func (database *Database) Rollback(t *testing.T) {
	t.Helper()

	if database.rolledBack {
		return
	}
	if _, err := database.provider.DownTo(database.ctx, 0); err != nil {
		t.Fatalf("roll back migrations: %v", err)
	}
	database.rolledBack = true
}

func migrationFiles(t *testing.T) fs.FS {
	t.Helper()

	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate PostgreSQL test harness")
	}

	return os.DirFS(filepath.Join(filepath.Dir(filename), "..", "..", "..", "db", "migrations"))
}

func randomHex(t *testing.T, byteCount int) string {
	t.Helper()

	buffer := make([]byte, byteCount)
	if _, err := rand.Read(buffer); err != nil {
		t.Fatalf("generate test schema suffix: %v", err)
	}

	return hex.EncodeToString(buffer)
}
