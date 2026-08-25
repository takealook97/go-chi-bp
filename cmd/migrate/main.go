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
const defaultTimeout = 5 * time.Minute

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	// The migration job reads only what it uses. Loading the application
	// configuration would fail a deployment on an HTTP or CORS value that no
	// migration reads, and report it as a schema failure.
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return errors.New("DATABASE_URL must not be empty")
	}

	timeout, err := migrationTimeout()
	if err != nil {
		return err
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	applied, err := database.Migrate(ctx, databaseURL, db.Migrations())
	if err != nil {
		return err
	}

	logger.Info("migrations applied", "count", len(applied), "versions", applied)

	return nil
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
