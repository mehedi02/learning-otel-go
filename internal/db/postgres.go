package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/XSAM/otelsql"
	_ "github.com/jackc/pgx/v5/stdlib"
	semconv "go.opentelemetry.io/otel/semconv/v1.30.0"

	"github.com/mehedi/user-service-go/internal/config"
)

func NewPostgres(cfg *config.Config, log *slog.Logger) (*sql.DB, error) {
	dsn := fmt.Sprintf(
		"postgres://%s:%s@%s:%s/%s",
		cfg.PostgresUser, cfg.PostgresPassword, cfg.PostgresHost, cfg.PostgresPort, cfg.PostgresDB,
	)

	port, _ := strconv.Atoi(cfg.PostgresPort)

	// otelsql wraps the underlying pgx driver so every Exec/Query/Begin/etc.
	// emits a child span using the ctx the caller passes through. Static
	// attributes here become low-cardinality dimensions on every db span.
	db, err := otelsql.Open("pgx", dsn,
		otelsql.WithAttributes(
			semconv.DBSystemNamePostgreSQL,
			semconv.DBNamespace(cfg.PostgresDB),
			semconv.ServerAddress(cfg.PostgresHost),
			semconv.ServerPort(port),
		),
		otelsql.WithSpanOptions(otelsql.SpanOptions{
			// Quiet by default; only the query/exec spans are interesting.
			Ping:                 false, // no span for startup ping
			RowsNext:             false, // no event-per-row noise
			DisableErrSkip:       true,  // suppress driver.ErrSkip "fall back" non-errors
			OmitConnectorConnect: true,  // skip per-connection-checkout spans
			OmitConnResetSession: true,  // skip pool-reset spans
			OmitConnPrepare:      true,  // pgx auto-prepares; would flood
			OmitRows:             true,  // skip the wrapping "rows" span; query span is enough
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("postgres: open: %w", err)
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}

	// Register connection-pool metrics (open/idle/in-use, wait counts/durations).
	// The returned Registration's Unregister is best-effort; the process exit
	// will tear it down anyway, so we don't track it here.
	if _, err := otelsql.RegisterDBStatsMetrics(db,
		otelsql.WithAttributes(
			semconv.DBSystemNamePostgreSQL,
			semconv.DBNamespace(cfg.PostgresDB),
		),
	); err != nil {
		return nil, fmt.Errorf("postgres: register db stats metrics: %w", err)
	}

	log.Info("connected to postgres",
		"host", cfg.PostgresHost,
		"port", cfg.PostgresPort,
		"db", cfg.PostgresDB,
		"user", cfg.PostgresUser,
	)

	return db, nil
}

func RunMigrations(ctx context.Context, db *sql.DB, log *slog.Logger) error {
	query, err := os.ReadFile("migrations/001_users.sql")
	if err != nil {
		return fmt.Errorf("read migrations file: %w", err)
	}

	if _, err := db.ExecContext(ctx, string(query)); err != nil {
		return fmt.Errorf("execute migrations: %w", err)
	}

	log.Info("applied migrations", "count", 1)
	return nil
}
