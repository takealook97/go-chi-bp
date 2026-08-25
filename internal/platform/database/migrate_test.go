package database

import (
	"context"
	"strings"
	"testing"
	"testing/fstest"
)

func TestMigrateRejectsInvalidURL(t *testing.T) {
	t.Parallel()

	_, err := Migrate(context.Background(), "://invalid", fstest.MapFS{})
	if err == nil || !strings.Contains(err.Error(), "parse database configuration") {
		t.Fatalf("Migrate() error = %v, want configuration parsing error", err)
	}
}

// A migrator holding no migrations would report a database as fully migrated
// while applying nothing, which is the one failure a deployment gate must not
// pass silently.
func TestMigrateRejectsAnEmptyMigrationSet(t *testing.T) {
	t.Parallel()

	_, err := Migrate(context.Background(), "postgres://user@localhost:5432/app", fstest.MapFS{})
	if err == nil || !strings.Contains(err.Error(), "create migration provider") {
		t.Fatalf("Migrate() error = %v, want migration provider error", err)
	}
}
