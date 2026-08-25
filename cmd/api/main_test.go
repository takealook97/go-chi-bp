package main

import (
	"context"
	"log/slog"
	"strings"
	"testing"
)

// The development level exists so that a local run shows debug records without
// a configuration change, and so that no other environment does.
func TestNewLoggerEnablesDebugOnlyInDevelopment(t *testing.T) {
	t.Parallel()

	for name, testCase := range map[string]struct {
		environment string
		wantDebug   bool
	}{
		"development": {environment: "development", wantDebug: true},
		"staging":     {environment: "staging", wantDebug: false},
		"production":  {environment: "production", wantDebug: false},
		"unset":       {environment: "", wantDebug: false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			logger := newLogger(testCase.environment)

			if got := logger.Enabled(context.Background(), slog.LevelDebug); got != testCase.wantDebug {
				t.Fatalf("debug enabled = %t, want %t", got, testCase.wantDebug)
			}
			if !logger.Enabled(context.Background(), slog.LevelInfo) {
				t.Fatal("info records are disabled, so the process would start silently")
			}
		})
	}
}

// Configuration is validated before anything is opened, so an unusable value
// stops the process at startup rather than at the first request that needs it.
func TestRunRejectsUnusableConfiguration(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	err := run()

	if err == nil {
		t.Fatal("run() error = nil, want an error")
	}
	if !strings.Contains(err.Error(), "load configuration") {
		t.Fatalf("run() error = %v, want it to report a configuration failure", err)
	}
}
