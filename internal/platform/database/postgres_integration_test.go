//go:build integration

package database

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/lukuku-dev/go-chi-bp/internal/platform/config"
)

func TestOpenIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	pool, err := Open(ctx, config.Database{
		URL:              databaseURL,
		MaxConnections:   4,
		MinConnections:   1,
		MaxConnLifetime:  time.Minute,
		MaxConnIdleTime:  time.Minute,
		StatementTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("Open() unexpected error: %v", err)
	}
	defer pool.Close()

	if pool.Config().MaxConns != 4 || pool.Config().MinConns != 1 {
		t.Fatalf("pool bounds = %d..%d, want 1..4", pool.Config().MinConns, pool.Config().MaxConns)
	}
	if err := pool.Ping(ctx); err != nil {
		t.Fatalf("Ping() unexpected error: %v", err)
	}
}

// queryCanceledCode is the SQLSTATE PostgreSQL returns when statement_timeout
// cancels a statement.
const queryCanceledCode = "57014"

// A statement that outruns its limit must be cancelled by PostgreSQL rather than
// hold its pooled connection until the client goes away.
func TestOpenBoundsStatementDurationIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()

	pool, err := Open(ctx, config.Database{
		URL:              databaseURL,
		MaxConnections:   2,
		MinConnections:   1,
		MaxConnLifetime:  time.Minute,
		MaxConnIdleTime:  time.Minute,
		StatementTimeout: 250 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Open() unexpected error: %v", err)
	}
	defer pool.Close()

	_, err = pool.Exec(ctx, "SELECT pg_sleep(3)")

	var pgError *pgconn.PgError
	if !errors.As(err, &pgError) || pgError.Code != queryCanceledCode {
		t.Fatalf("Exec() error = %v, want SQLSTATE %s", err, queryCanceledCode)
	}
	if ctx.Err() != nil {
		t.Fatal("the statement outlived its limit and was stopped by the caller's context instead")
	}
}
