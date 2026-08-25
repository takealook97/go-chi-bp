package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

// Migrate applies every pending migration and returns the versions it applied.
//
// Migrations run on a connection of their own rather than the application pool.
// The job that applies them runs before the process that serves traffic exists,
// and it must be able to fail a deployment on its own: a schema change that
// never ran and one that ran are different states, and only the migrator that
// owns the attempt can report which happened.
//
// This is the same Goose provider the integration harness runs against a real
// database, so what CI verifies and what a deployment applies are one code path.
func Migrate(ctx context.Context, databaseURL string, migrations fs.FS) ([]int64, error) {
	connConfig, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database configuration: %w", err)
	}

	migrationDatabase := stdlib.OpenDB(*connConfig)

	provider, err := goose.NewProvider(goose.DialectPostgres, migrationDatabase, migrations)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("create migration provider: %w", err), closeMigrationDatabase(migrationDatabase))
	}

	results, err := provider.Up(ctx)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("apply migrations: %w", err), closeMigrationDatabase(migrationDatabase))
	}

	applied := make([]int64, 0, len(results))
	for _, result := range results {
		applied = append(applied, result.Source.Version)
	}

	return applied, closeMigrationDatabase(migrationDatabase)
}

func closeMigrationDatabase(migrationDatabase *sql.DB) error {
	if err := migrationDatabase.Close(); err != nil {
		return fmt.Errorf("close migration database: %w", err)
	}

	return nil
}
