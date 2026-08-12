package database

import (
	"context"
	"strings"
	"testing"

	"github.com/lukuku-dev/go-chi-bp/internal/platform/config"
)

func TestOpenRejectsInvalidURL(t *testing.T) {
	t.Parallel()

	_, err := Open(context.Background(), config.Database{URL: "://invalid"})
	if err == nil || !strings.Contains(err.Error(), "parse database configuration") {
		t.Fatalf("Open() error = %v, want configuration parsing error", err)
	}
}
