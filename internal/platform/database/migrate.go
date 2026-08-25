package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// MigrationOptions bound and observe one migration run.
type MigrationOptions struct {
	// LockTimeout bounds how long a migration waits for a lock it cannot take.
	// Without it a schema change that meets an open transaction waits for the
	// whole run's timeout, and every query needing that table queues behind it,
	// because PostgreSQL grants lock requests in order. Zero waits forever.
	LockTimeout time.Duration
	// Logger receives Goose's per-migration progress, which otherwise goes to
	// the standard logger as plain text and cannot be read by a JSON collector.
	Logger *slog.Logger
}

// Migrate applies every pending migration and returns the versions it applied.
// A partial failure returns both: the versions that were applied and the error,
// because an operator deciding what to do next needs to know how far it got.
//
// Migrations run on a connection of their own rather than the application pool.
// The job that applies them runs before the process that serves traffic exists,
// and it must be able to fail a deployment on its own.
//
// This is the same Goose provider the integration harness runs against a real
// database, so what CI verifies and what a deployment applies are one code path.
func Migrate(ctx context.Context, databaseURL string, migrations fs.FS, options MigrationOptions) ([]int64, error) {
	connConfig, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database configuration: %w", err)
	}

	// A schema change may legitimately run for minutes, so the statement limit
	// the application sets for request handling must not be inherited from a
	// role default here.
	connConfig.RuntimeParams["statement_timeout"] = "0"
	if options.LockTimeout > 0 {
		connConfig.RuntimeParams["lock_timeout"] = strconv.FormatInt(options.LockTimeout.Milliseconds(), 10)
	}

	migrationDatabase := stdlib.OpenDB(*connConfig)

	providerOptions := []goose.ProviderOption{}
	if options.Logger != nil {
		providerOptions = append(providerOptions, goose.WithSlog(options.Logger))
	}

	provider, err := goose.NewProvider(goose.DialectPostgres, migrationDatabase, migrations, providerOptions...)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("create migration provider: %w", err), closeMigrationDatabase(migrationDatabase))
	}

	results, err := provider.Up(ctx)
	applied := appliedVersions(results)
	if err != nil {
		var partial *goose.PartialError
		if errors.As(err, &partial) {
			applied = appliedVersions(partial.Applied)
		}

		return applied, errors.Join(fmt.Errorf("apply migrations: %w", err), closeMigrationDatabase(migrationDatabase))
	}

	return applied, closeMigrationDatabase(migrationDatabase)
}

// appliedVersions returns the version of every migration that ran.
func appliedVersions(results []*goose.MigrationResult) []int64 {
	applied := make([]int64, 0, len(results))
	for _, result := range results {
		applied = append(applied, result.Source.Version)
	}

	return applied
}

func closeMigrationDatabase(migrationDatabase *sql.DB) error {
	if err := migrationDatabase.Close(); err != nil {
		return fmt.Errorf("close migration database: %w", err)
	}

	return nil
}
