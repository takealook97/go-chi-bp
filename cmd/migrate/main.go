// Package main applies pending database migrations and exits.
//
// This runs as a deployment step of its own, before the process that serves
// traffic starts. Applying migrations at startup would tie a schema change to
// whichever replica happened to boot first, run it concurrently on every other
// replica, and leave a failed change to be discovered as a crash loop.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/lukuku-dev/go-chi-bp/db"
	"github.com/lukuku-dev/go-chi-bp/internal/platform/database"
)

// defaultTimeout bounds one migration run. A migration that outlives it is
// cancelled rather than left holding locks the running application waits on.
const (
	defaultTimeout = 5 * time.Minute
	// Failing fast is better than blocking every reader of the table: a
	// migration that cannot take its lock in this long is queued behind
	// something the deployment should surface, not wait out. Set
	// MIGRATE_LOCK_TIMEOUT to 0 to wait indefinitely.
	defaultLockTimeout = 5 * time.Second
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	applied, err := run(logger)
	if err != nil {
		// One record per outcome. The versions that did land decide what an
		// operator does next, and a second plain line for the same failure only
		// gives a log collector an orphaned event to file.
		logger.Error("migrations failed", "error", err.Error(), "applied", applied)
		os.Exit(1)
	}

	logger.Info("migrations applied", "count", len(applied), "versions", applied)
}

func run(logger *slog.Logger) ([]int64, error) {
	// The migration job reads only what it uses. Loading the application
	// configuration would fail a deployment on an HTTP or CORS value that no
	// migration reads, and report it as a schema failure.
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return nil, errors.New("DATABASE_URL must not be empty")
	}

	timeout, err := migrationTimeout()
	if err != nil {
		return nil, err
	}

	lockTimeout, err := migrationLockTimeout()
	if err != nil {
		return nil, err
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return database.Migrate(ctx, databaseURL, db.Migrations(), database.MigrationOptions{
		LockTimeout: lockTimeout,
		Logger:      logger,
	})
}

// migrationLockTimeout reads how long a migration may wait for a lock. A schema
// change that meets an open transaction otherwise waits for the whole run's
// timeout while every query needing that table queues behind it.
func migrationLockTimeout() (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv("MIGRATE_LOCK_TIMEOUT"))
	if value == "" {
		return defaultLockTimeout, nil
	}

	timeout, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse MIGRATE_LOCK_TIMEOUT: %w", err)
	}
	if timeout < 0 {
		return 0, errors.New("MIGRATE_LOCK_TIMEOUT must not be negative")
	}

	return timeout, nil
}

// migrationTimeout reads how long one migration run may take. Schema changes
// that rewrite large tables outlive any default worth setting for everyone else.
func migrationTimeout() (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv("MIGRATE_TIMEOUT"))
	if value == "" {
		return defaultTimeout, nil
	}

	timeout, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse MIGRATE_TIMEOUT: %w", err)
	}
	if timeout <= 0 {
		return 0, errors.New("MIGRATE_TIMEOUT must be positive")
	}

	return timeout, nil
}
