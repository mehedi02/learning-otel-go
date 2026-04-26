package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/mehedi/user-service-go/internal/config"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func NewPostgres(cfg *config.Config, log *slog.Logger) (*sql.DB, error) {
	dsn := fmt.Sprintf("postgres://%s:%s@%s:%s/%s", cfg.PostgresUser, cfg.PostgresPassword, cfg.PostgresHost, cfg.PostgresPort, cfg.PostgresDB)

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		return nil, err
	}

	log.Info("connected to postgres", "dsn", dsn)

	return db, nil
}

func RunMigrations(db *sql.DB, log *slog.Logger) error {
	query, err := os.ReadFile("migrations/001_users.sql")
	if err != nil{
		return fmt.Errorf("failed to read migrations file: %w", err)
	}

	_, err = db.Exec(string(query))
	if err != nil {
		return fmt.Errorf("failed to execute migrations: %w", err)
	}

	log.Info("applied migrations", "count", 1)

	return nil
}