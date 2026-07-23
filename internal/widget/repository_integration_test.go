package widget

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	dbgen "github.com/lukuku-dev/go-chi-bp/internal/database/sqlc"
)

func TestPostgresRepositoryIntegration(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	schema := "widget_test_" + randomHex(t, 8)
	quotedSchema := pgx.Identifier{schema}.Sanitize()
	basePool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect to integration database: %v", err)
	}
	defer basePool.Close()

	if _, err := basePool.Exec(ctx, "CREATE SCHEMA "+quotedSchema); err != nil {
		t.Fatalf("create isolated test schema: %v", err)
	}
	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cleanupCancel()

		if _, cleanupErr := basePool.Exec(cleanupCtx, "DROP SCHEMA "+quotedSchema+" CASCADE"); cleanupErr != nil {
			t.Errorf("drop isolated test schema: %v", cleanupErr)
		}
	}()

	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse integration database configuration: %v", err)
	}
	poolConfig.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatalf("connect to isolated test schema: %v", err)
	}
	defer pool.Close()

	upMigration, downMigration := readWidgetMigration(t)
	if _, err := pool.Exec(ctx, upMigration); err != nil {
		t.Fatalf("apply widget migration: %v", err)
	}

	repository := NewPostgresRepository(dbgen.New(pool))
	created, err := repository.Create(ctx, "integration widget")
	if err != nil {
		t.Fatalf("Create() unexpected error: %v", err)
	}
	if created.ID < 1 || created.Name != "integration widget" {
		t.Fatalf("Create() = %+v, want persisted widget", created)
	}
	if created.CreatedAt.Location() != time.UTC || created.UpdatedAt.Location() != time.UTC {
		t.Fatalf("Create() timestamps are not normalized to UTC: %+v", created)
	}

	selected, err := repository.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if selected != created {
		t.Fatalf("Get() = %+v, want %+v", selected, created)
	}

	items, err := repository.List(ctx, ListOptions{Limit: 10})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("List() = %+v, want created widget", items)
	}

	if err := repository.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}
	if _, err := repository.Get(ctx, created.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get() after delete error = %v, want ErrNotFound", err)
	}

	if _, err := pool.Exec(ctx, downMigration); err != nil {
		t.Fatalf("roll back widget migration: %v", err)
	}
	var tableName *string
	if err := pool.QueryRow(ctx, "SELECT to_regclass('widgets')::text").Scan(&tableName); err != nil {
		t.Fatalf("check rolled-back migration: %v", err)
	}
	if tableName != nil {
		t.Fatalf("widgets table still exists after down migration: %q", *tableName)
	}
}

func randomHex(t *testing.T, byteCount int) string {
	t.Helper()

	buffer := make([]byte, byteCount)
	if _, err := rand.Read(buffer); err != nil {
		t.Fatalf("generate test schema suffix: %v", err)
	}

	return hex.EncodeToString(buffer)
}

func readWidgetMigration(t *testing.T) (string, string) {
	t.Helper()

	contents, err := os.ReadFile("../../db/migrations/00001_create_widgets.sql")
	if err != nil {
		t.Fatalf("read widget migration: %v", err)
	}

	afterUp, found := strings.CutPrefix(string(contents), "-- +goose Up")
	if !found {
		t.Fatal("widget migration is missing the Goose Up marker")
	}
	upMigration, downMigration, found := strings.Cut(afterUp, "-- +goose Down")
	if !found || strings.TrimSpace(upMigration) == "" || strings.TrimSpace(downMigration) == "" {
		t.Fatal("widget migration must contain non-empty up and down sections")
	}

	return upMigration, downMigration
}
