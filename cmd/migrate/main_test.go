package main

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

// These tests set process environment variables, so they must not run in
// parallel with each other or with anything else reading the same variables.

func TestMigrationTimeout(t *testing.T) {
	for name, testCase := range map[string]struct {
		value     string
		want      time.Duration
		wantError string
	}{
		"empty falls back":      {value: "", want: defaultTimeout},
		"blank falls back":      {value: "   ", want: defaultTimeout},
		"explicit value":        {value: "90s", want: 90 * time.Second},
		"unparsable":            {value: "later", wantError: "parse MIGRATE_TIMEOUT"},
		"zero is not a run":     {value: "0s", wantError: "must be positive"},
		"negative is not a run": {value: "-1s", wantError: "must be positive"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("MIGRATE_TIMEOUT", testCase.value)

			got, err := migrationTimeout()

			assertDuration(t, "migrationTimeout", got, err, testCase.want, testCase.wantError)
		})
	}
}

func TestMigrationLockTimeout(t *testing.T) {
	for name, testCase := range map[string]struct {
		value     string
		want      time.Duration
		wantError string
	}{
		"empty falls back": {value: "", want: defaultLockTimeout},
		"explicit value":   {value: "2s", want: 2 * time.Second},
		// Zero is documented as waiting indefinitely, so unlike the run timeout
		// it is a valid setting rather than a configuration error.
		"zero waits forever": {value: "0s", want: 0},
		"unparsable":         {value: "soon", wantError: "parse MIGRATE_LOCK_TIMEOUT"},
		"negative":           {value: "-1s", wantError: "must not be negative"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("MIGRATE_LOCK_TIMEOUT", testCase.value)

			got, err := migrationLockTimeout()

			assertDuration(t, "migrationLockTimeout", got, err, testCase.want, testCase.wantError)
		})
	}
}

// A migration job that starts without a database URL must fail before it opens
// anything, because the alternative is a deployment that reports success while
// applying nothing.
func TestRunRejectsAnEmptyDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	applied, err := run(slog.New(slog.DiscardHandler))

	if err == nil {
		t.Fatal("run() error = nil, want an error")
	}
	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf("run() error = %v, want it to name DATABASE_URL", err)
	}
	if applied != nil {
		t.Fatalf("run() applied = %v, want no versions", applied)
	}
}

// An unusable timeout must be reported as the configuration error it is, before
// the job connects to anything.
func TestRunReportsAnUnusableTimeout(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://user@localhost:5432/app")
	t.Setenv("MIGRATE_TIMEOUT", "never")

	if _, err := run(slog.New(slog.DiscardHandler)); err == nil ||
		!strings.Contains(err.Error(), "MIGRATE_TIMEOUT") {
		t.Fatalf("run() error = %v, want it to name MIGRATE_TIMEOUT", err)
	}
}

func assertDuration(t *testing.T, name string, got time.Duration, err error, want time.Duration, wantError string) {
	t.Helper()

	if wantError != "" {
		if err == nil || !strings.Contains(err.Error(), wantError) {
			t.Fatalf("%s() error = %v, want it to contain %q", name, err, wantError)
		}

		return
	}
	if err != nil {
		t.Fatalf("%s() unexpected error: %v", name, err)
	}
	if got != want {
		t.Fatalf("%s() = %v, want %v", name, got, want)
	}
}
