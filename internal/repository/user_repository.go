package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/mehedi/user-service-go/internal/models"
)

type UserRepository interface {
	Create(ctx context.Context, name, email string) (*models.User, error)
	FindByID(ctx context.Context, id int) (*models.User, error)
	Update(ctx context.Context, id int, name, email string) (*models.User, error)
	Delete(ctx context.Context, id int) error
	FindAll(ctx context.Context) ([]models.User, error)
}

type postgresUserRepository struct {
	db *sql.DB
}

func NewPostgresUserRepository(db *sql.DB) UserRepository {
	return &postgresUserRepository{db: db}
}

func (r *postgresUserRepository) Create(ctx context.Context, name, email string) (*models.User, error) {
	query := "INSERT INTO users (name, email) VALUES ($1, $2) RETURNING id, name, email, created_at, updated_at"
	var user models.User
	err := r.db.QueryRowContext(ctx, query, name, email).Scan(&user.ID, &user.Name, &user.Email, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}
	return &user, nil
}

func (r *postgresUserRepository) FindByID(ctx context.Context, id int) (*models.User, error) {
	query := "SELECT id, name, email, created_at, updated_at FROM users WHERE id = $1"
	var user models.User
	err := r.db.QueryRowContext(ctx, query, id).Scan(&user.ID, &user.Name, &user.Email, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, models.ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to find user by id: %w", err)
	}
	return &user, nil
}

func (r *postgresUserRepository) Update(ctx context.Context, id int, name, email string) (*models.User, error) {
	query := "UPDATE users SET name = $1, email = $2, updated_at = CURRENT_TIMESTAMP WHERE id = $3 RETURNING id, name, email, created_at, updated_at"
	var user models.User
	err := r.db.QueryRowContext(ctx, query, name, email, id).Scan(&user.ID, &user.Name, &user.Email, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, models.ErrUserNotFound
		}
		return nil, fmt.Errorf("failed to update user: %w", err)
	}
	return &user, nil
}

func (r *postgresUserRepository) Delete(ctx context.Context, id int) error {
	query := "DELETE FROM users WHERE id = $1"
	result, err := r.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete user: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return models.ErrUserNotFound
	}
	return nil
}

func (r *postgresUserRepository) FindAll(ctx context.Context) ([]models.User, error) {
	query := "SELECT id, name, email, created_at, updated_at FROM users"
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to find all users: %w", err)
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan user: %w", err)
		}
		users = append(users, u)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration error: %w", err)
	}

	return users, nil
}