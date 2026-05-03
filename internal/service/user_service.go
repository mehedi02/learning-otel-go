package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"

	"github.com/mehedi/user-service-go/internal/cache"
	"github.com/mehedi/user-service-go/internal/models"
	"github.com/mehedi/user-service-go/internal/repository"
)

const cacheTTL = 1 * time.Hour

// Package-level tracer following OTel convention: instrumentation scope = package import path
var tracer = otel.Tracer("github.com/mehedi/user-service-go/internal/service")

type UserService interface {
	CreateUser(ctx context.Context, name, email string) (*models.User, error)
	GetUser(ctx context.Context, id int) (*models.User, error)
	UpdateUser(ctx context.Context, id int, name, email string) (*models.User, error)
	DeleteUser(ctx context.Context, id int) error
	GetAllUsers(ctx context.Context) ([]models.User, error)
}

type userService struct {
	repo  repository.UserRepository
	cache *cache.Cache
	log   *slog.Logger
}

func NewUserService(repo repository.UserRepository, cache *cache.Cache, log *slog.Logger) UserService {
	return &userService{repo: repo, cache: cache, log: log}
}

func cacheKey(id int) string {
	return fmt.Sprintf("user:%d", id)
}

func (s *userService) CreateUser(ctx context.Context, name, email string) (*models.User, error) {
	ctx, span := tracer.Start(ctx, "user.create")
	defer span.End()

	user, err := s.repo.Create(ctx, name, email)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "create user failed")
		return nil, err
	}

	// Cache write is best-effort; failure doesn't fail the operation
	if err := s.cache.Set(ctx, cacheKey(user.ID), user, cacheTTL); err != nil {
		s.log.WarnContext(ctx, "failed to cache user after create", "user_id", user.ID, "error", err)
		span.SetAttributes(attribute.Bool("cache.write_error", true))
	}

	// Success attributes known only after creation
	span.SetAttributes(attribute.Int("app.user.id", user.ID))

	s.log.InfoContext(ctx, "user created", "user_id", user.ID)
	return user, nil
}

func (s *userService) GetUser(ctx context.Context, id int) (*models.User, error) {
	ctx, span := tracer.Start(ctx, "user.get",
		trace.WithAttributes(
			attribute.Int("app.user.id", id),
		),
	)
	defer span.End()

	var user models.User
	found, err := s.cache.Get(ctx, cacheKey(id), &user)
	if err != nil {
		s.log.WarnContext(ctx, "cache get failed, falling back to db", "user_id", id, "error", err)
		span.SetAttributes(attribute.Bool("cache.read_error", true))
	}

	// Critical attribute: cache.hit enables cache hit rate queries in TraceQL
	span.SetAttributes(attribute.Bool("cache.hit", found))
	if found {
		s.log.InfoContext(ctx, "user found in cache", "user_id", id)
		return &user, nil
	}

	dbUser, err := s.repo.FindByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "get user failed")
		return nil, err
	}

	if err := s.cache.Set(ctx, cacheKey(id), dbUser, cacheTTL); err != nil {
		s.log.WarnContext(ctx, "failed to cache user after db fetch", "user_id", id, "error", err)
		span.SetAttributes(attribute.Bool("cache.write_error", true))
	}

	s.log.InfoContext(ctx, "user fetched from db", "user_id", id)
	return dbUser, nil
}

func (s *userService) UpdateUser(ctx context.Context, id int, name, email string) (*models.User, error) {
	ctx, span := tracer.Start(ctx, "user.update",
		trace.WithAttributes(
			attribute.Int("app.user.id", id),
		),
	)
	defer span.End()

	user, err := s.repo.Update(ctx, id, name, email)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "update user failed")
		return nil, err
	}

	if err := s.cache.Set(ctx, cacheKey(id), user, cacheTTL); err != nil {
		s.log.WarnContext(ctx, "failed to update cache after update", "user_id", id, "error", err)
		span.SetAttributes(attribute.Bool("cache.write_error", true))
	}

	s.log.InfoContext(ctx, "user updated", "user_id", id)
	return user, nil
}

func (s *userService) DeleteUser(ctx context.Context, id int) error {
	ctx, span := tracer.Start(ctx, "user.delete",
		trace.WithAttributes(
			attribute.Int("app.user.id", id),
		),
	)
	defer span.End()

	if err := s.repo.Delete(ctx, id); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "delete user failed")
		return err
	}

	if err := s.cache.Del(ctx, cacheKey(id)); err != nil {
		s.log.WarnContext(ctx, "failed to delete cache after delete", "user_id", id, "error", err)
		span.SetAttributes(attribute.Bool("cache.delete_error", true))
	}

	s.log.InfoContext(ctx, "user deleted", "user_id", id)
	return nil
}

func (s *userService) GetAllUsers(ctx context.Context) ([]models.User, error) {
	ctx, span := tracer.Start(ctx, "user.list")
	defer span.End()

	users, err := s.repo.FindAll(ctx)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "list users failed")
		return nil, err
	}

	// Success attribute: result count is searchable in TraceQL
	span.SetAttributes(attribute.Int("app.user.count", len(users)))

	s.log.InfoContext(ctx, "fetched all users", "count", len(users))
	return users, nil
}