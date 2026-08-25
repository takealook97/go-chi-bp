package database

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/pressly/goose/v3"
)

func TestMigrateRejectsInvalidURL(t *testing.T) {
	t.Parallel()

	_, err := Migrate(context.Background(), "://invalid", fstest.MapFS{}, MigrationOptions{})
	if err == nil || !strings.Contains(err.Error(), "parse database configuration") {
		t.Fatalf("Migrate() error = %v, want configuration parsing error", err)
	}
}

// A migrator holding no migrations would report a database as fully migrated
// while applying nothing, which is the one failure a deployment gate must not
// pass silently.
func TestMigrateRejectsAnEmptyMigrationSet(t *testing.T) {
	t.Parallel()

	_, err := Migrate(context.Background(), "postgres://user@localhost:5432/app", fstest.MapFS{}, MigrationOptions{})
	// Matching the sentinel rather than this package's wrapper text keeps the
	// test tied to the condition: any other provider failure would satisfy a
	// string match while proving nothing about empty migration sets.
	if !errors.Is(err, goose.ErrNoMigrations) {
		t.Fatalf("Migrate() error = %v, want goose.ErrNoMigrations", err)
	}
	if !strings.Contains(err.Error(), "create migration provider") {
		t.Errorf("Migrate() error = %v, want it wrapped with operation context", err)
	}
}
