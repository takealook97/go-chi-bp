//go:build integration

package database

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lukuku-dev/go-chi-bp/db"
)

// The deployment applies the embedded migrations to a database that has none, so
// that is what this exercises: an empty schema, the real migration set, and a
// second run that must add nothing. Re-running a migration job is ordinary in a
// retried deployment, and a job that applied anything twice would fail there
// rather than here.
func TestMigrateAppliesEveryMigrationOnceIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	schemaURL := emptySchema(ctx, t, databaseURL)

	applied, err := Migrate(ctx, schemaURL, db.Migrations())
	if err != nil {
		t.Fatalf("Migrate() unexpected error: %v", err)
	}
	if len(applied) == 0 {
		t.Fatal("Migrate() applied no migrations to an empty schema")
	}
	for index, version := range applied {
		if index > 0 && version <= applied[index-1] {
			t.Fatalf("Migrate() applied versions %v out of order", applied)
		}
	}

	reapplied, err := Migrate(ctx, schemaURL, db.Migrations())
	if err != nil {
		t.Fatalf("Migrate() unexpected error on the second run: %v", err)
	}
	if len(reapplied) != 0 {
		t.Fatalf("Migrate() applied %v a second time", reapplied)
	}
}

// emptySchema creates an isolated schema, removes it when the test ends, and
// returns a URL whose connections resolve to it.
func emptySchema(ctx context.Context, t *testing.T, databaseURL string) string {
	t.Helper()

	suffix := make([]byte, 8)
	if _, err := rand.Read(suffix); err != nil {
		t.Fatalf("generate schema suffix: %v", err)
	}
	schema := "migrate_test_" + hex.EncodeToString(suffix)
	quotedSchema := pgx.Identifier{schema}.Sanitize()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to integration database: %v", err)
	}
	if _, err := pool.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		pool.Close()
		t.Fatalf("create isolated test schema: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cleanupCancel()
		if _, err := pool.Exec(cleanupCtx, "DROP SCHEMA "+quotedSchema+" CASCADE"); err != nil {
			t.Errorf("drop isolated test schema: %v", err)
		}
		pool.Close()
	})

	separator := "?"
	if strings.Contains(databaseURL, "?") {
		separator = "&"
	}

	return databaseURL + separator + "search_path=" + schema
}
