// Package database constructs shared database infrastructure.
package database

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/lukuku-dev/go-chi-bp/internal/platform/config"
)

const (
	// poolHealthCheckPeriod is how often the pool checks an idle connection.
	poolHealthCheckPeriod = 30 * time.Second
	// startupPingTimeout bounds the one round trip that proves the configured
	// database is reachable before the process starts serving.
	startupPingTimeout = 3 * time.Second
)

// Open creates and verifies a PostgreSQL connection pool.
func Open(ctx context.Context, cfg config.Database) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse database configuration: %w", err)
	}

	poolConfig.MaxConns = cfg.MaxConnections
	poolConfig.MinConns = cfg.MinConnections
	poolConfig.MaxConnLifetime = cfg.MaxConnLifetime
	poolConfig.MaxConnIdleTime = cfg.MaxConnIdleTime
	poolConfig.HealthCheckPeriod = poolHealthCheckPeriod
	// Bound statements on the server so a slow query releases its pooled
	// connection on its own. The caller's context cannot be relied on for this:
	// an HTTP request carries no deadline of its own, and it is cancelled only
	// once the client gives up.
	poolConfig.ConnConfig.RuntimeParams["statement_timeout"] = strconv.FormatInt(cfg.StatementTimeout.Milliseconds(), 10)

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, startupPingTimeout)
	defer cancel()

	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()

		return nil, fmt.Errorf("ping database: %w", err)
	}

	return pool, nil
}
