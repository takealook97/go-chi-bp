// Package widgetpostgres persists widgets in PostgreSQL.
package widgetpostgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/lukuku-dev/go-chi-bp/internal/widget"
	dbgen "github.com/lukuku-dev/go-chi-bp/internal/widget/widgetpostgres/dbgen"
)

// PostgresRepository persists widgets in PostgreSQL.
type PostgresRepository struct {
	queries *dbgen.Queries
}

// NewPostgresRepository constructs a PostgreSQL widget repository.
func NewPostgresRepository(queries *dbgen.Queries) *PostgresRepository {
	if queries == nil {
		panic("database queries must not be nil")
	}

	return &PostgresRepository{queries: queries}
}

// Create inserts a widget.
func (repository *PostgresRepository) Create(ctx context.Context, name string) (widget.Widget, error) {
	row, err := repository.queries.CreateWidget(ctx, name)
	if err != nil {
		return widget.Widget{}, fmt.Errorf("insert widget: %w", err)
	}

	return widgetFromDatabase(row), nil
}

// Get selects a widget by identifier.
func (repository *PostgresRepository) Get(ctx context.Context, id int64) (widget.Widget, error) {
	row, err := repository.queries.GetWidget(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return widget.Widget{}, widget.ErrNotFound
	}
	if err != nil {
		return widget.Widget{}, fmt.Errorf("select widget: %w", err)
	}

	return widgetFromDatabase(row), nil
}

// List selects a bounded page of widgets.
func (repository *PostgresRepository) List(ctx context.Context, options widget.ListOptions) ([]widget.Widget, error) {
	var (
		rows []dbgen.Widget
		err  error
	)
	if options.Cursor == nil {
		rows, err = repository.queries.ListWidgets(ctx, options.Limit)
	} else {
		rows, err = repository.queries.ListWidgetsAfter(ctx, dbgen.ListWidgetsAfterParams{
			CursorCreatedAt: options.Cursor.CreatedAt,
			CursorID:        options.Cursor.ID,
			PageLimit:       options.Limit,
		})
	}
	if err != nil {
		return nil, fmt.Errorf("select widgets: %w", err)
	}

	results := make([]widget.Widget, 0, len(rows))
	for _, row := range rows {
		results = append(results, widgetFromDatabase(row))
	}

	return results, nil
}

// Delete removes a widget by identifier.
func (repository *PostgresRepository) Delete(ctx context.Context, id int64) error {
	deletedRows, err := repository.queries.DeleteWidget(ctx, id)
	if err != nil {
		return fmt.Errorf("delete widget: %w", err)
	}
	if deletedRows == 0 {
		return widget.ErrNotFound
	}

	return nil
}

func widgetFromDatabase(row dbgen.Widget) widget.Widget {
	return widget.Widget{
		ID:        row.ID,
		Name:      row.Name,
		CreatedAt: row.CreatedAt.UTC(),
		UpdatedAt: row.UpdatedAt.UTC(),
	}
}
