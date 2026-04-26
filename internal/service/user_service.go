package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/mehedi/user-service-go/internal/cache"
	"github.com/mehedi/user-service-go/internal/models"
	"github.com/mehedi/user-service-go/internal/repository"
)

const cacheTTL = 1 * time.Hour

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
	user, err := s.repo.Create(ctx, name, email)
	if err != nil {
		return nil, err
	}

	if err := s.cache.Set(ctx, cacheKey(user.ID), user, cacheTTL); err != nil {
		s.log.Warn("failed to cache user after create", "user_id", user.ID, "error", err)
	}

	s.log.Info("user created", "user_id", user.ID)
	return user, nil
}

func (s *userService) GetUser(ctx context.Context, id int) (*models.User, error) {
	var user models.User
	found, err := s.cache.Get(ctx, cacheKey(id), &user)
	if err != nil {
		s.log.Warn("cache get failed, falling back to db", "user_id", id, "error", err)
	}
	if found {
		s.log.Info("user found in cache", "user_id", id)
		return &user, nil
	}

	dbUser, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if err := s.cache.Set(ctx, cacheKey(id), dbUser, cacheTTL); err != nil {
		s.log.Warn("failed to cache user after db fetch", "user_id", id, "error", err)
	}

	s.log.Info("user fetched from db", "user_id", id)
	return dbUser, nil
}

func (s *userService) UpdateUser(ctx context.Context, id int, name, email string) (*models.User, error) {
	user, err := s.repo.Update(ctx, id, name, email)
	if err != nil {
		return nil, err
	}

	if err := s.cache.Set(ctx, cacheKey(id), user, cacheTTL); err != nil {
		s.log.Warn("failed to update cache after update", "user_id", id, "error", err)
	}

	s.log.Info("user updated", "user_id", id)
	return user, nil
}

func (s *userService) DeleteUser(ctx context.Context, id int) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}

	if err := s.cache.Del(ctx, cacheKey(id)); err != nil {
		s.log.Warn("failed to delete cache after delete", "user_id", id, "error", err)
	}

	s.log.Info("user deleted", "user_id", id)
	return nil
}

func (s *userService) GetAllUsers(ctx context.Context) ([]models.User, error) {
	users, err := s.repo.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	s.log.Info("fetched all users", "count", len(users))
	return users, nil
}