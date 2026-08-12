package widgetpostgres

import (
	"testing"
	"time"

	dbgen "github.com/lukuku-dev/go-chi-bp/internal/widget/widgetpostgres/dbgen"
)

func TestWidgetFromDatabaseNormalizesTimestampsToUTC(t *testing.T) {
	t.Parallel()

	seoul := time.FixedZone("Asia/Seoul", 9*60*60)
	row := dbgen.Widget{
		ID:        1,
		Name:      "example",
		CreatedAt: time.Date(2026, time.July, 10, 15, 0, 0, 0, seoul),
		UpdatedAt: time.Date(2026, time.July, 10, 15, 0, 0, 0, seoul),
	}

	result := widgetFromDatabase(row)
	if result.CreatedAt.Location() != time.UTC {
		t.Fatalf("created location = %v, want UTC", result.CreatedAt.Location())
	}
	if result.UpdatedAt.Location() != time.UTC {
		t.Fatalf("updated location = %v, want UTC", result.UpdatedAt.Location())
	}
}
