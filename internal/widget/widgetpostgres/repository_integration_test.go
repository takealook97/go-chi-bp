//go:build integration

package widgetpostgres

import (
	"errors"
	"testing"
	"time"

	"github.com/lukuku-dev/go-chi-bp/internal/testkit/postgrestest"
	"github.com/lukuku-dev/go-chi-bp/internal/widget"
	dbgen "github.com/lukuku-dev/go-chi-bp/internal/widget/widgetpostgres/dbgen"
)

func TestPostgresRepositoryIntegration(t *testing.T) {
	database := postgrestest.New(t)
	ctx := t.Context()
	repository := NewPostgresRepository(dbgen.New(database.Pool))

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
	if _, err := database.Pool.Exec(ctx, "SELECT pg_sleep(0.01)"); err != nil {
		t.Fatalf("wait before update: %v", err)
	}
	var updatedAt time.Time
	if err := database.Pool.QueryRow(ctx, "UPDATE widgets SET name = $1 WHERE id = $2 RETURNING updated_at", "updated widget", created.ID).
		Scan(&updatedAt); err != nil {
		t.Fatalf("update widget timestamp: %v", err)
	}
	if !updatedAt.After(created.UpdatedAt) {
		t.Fatalf("updated_at = %v, want after %v", updatedAt, created.UpdatedAt)
	}

	second, err := repository.Create(ctx, "second widget")
	if err != nil {
		t.Fatalf("Create() second widget unexpected error: %v", err)
	}
	items, err := repository.List(ctx, widget.ListOptions{Limit: 1})
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(items) != 1 || items[0].ID != second.ID {
		t.Fatalf("List() = %+v, want second widget", items)
	}
	items, err = repository.List(ctx, widget.ListOptions{
		Limit:  1,
		Cursor: &widget.ListCursor{CreatedAt: items[0].CreatedAt, ID: items[0].ID},
	})
	if err != nil {
		t.Fatalf("List() after cursor unexpected error: %v", err)
	}
	if len(items) != 1 || items[0].ID != created.ID {
		t.Fatalf("List() after cursor = %+v, want first widget", items)
	}

	if err := repository.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete() unexpected error: %v", err)
	}
	if _, err := repository.Get(ctx, created.ID); !errors.Is(err, widget.ErrNotFound) {
		t.Fatalf("Get() after delete error = %v, want ErrNotFound", err)
	}
	if err := repository.Delete(ctx, second.ID); err != nil {
		t.Fatalf("Delete() second widget unexpected error: %v", err)
	}

	database.Rollback(t)
	var tableName *string
	if err := database.Pool.QueryRow(ctx, "SELECT to_regclass('widgets')::text").Scan(&tableName); err != nil {
		t.Fatalf("check rolled-back migration: %v", err)
	}
	if tableName != nil {
		t.Fatalf("widgets table still exists after rollback: %q", *tableName)
	}
}
