package handler

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/mehedi/user-service-go/internal/middleware"
	"github.com/mehedi/user-service-go/internal/models"
	"github.com/mehedi/user-service-go/internal/service"
)

type UserHandler struct {
	svc service.UserService
	log *slog.Logger
}

func NewUserHandler(svc service.UserService, log *slog.Logger) *UserHandler {
	return &UserHandler{svc: svc, log: log}
}

func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
	req, err := middleware.DecodeAndValidate(r, middleware.ValidateCreateUser)
	if err != nil {
		var ve middleware.ValidationErrors
		if errors.As(err, &ve) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"errors": ve})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"message": err.Error()}})
		return
	}

	user, err := h.svc.CreateUser(r.Context(), req.Name, req.Email)
	if err != nil {
		h.handleError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, user)
}

func (h *UserHandler) GetByID(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"message": "invalid user id"}})
		return
	}

	user, err := h.svc.GetUser(r.Context(), id)
	if err != nil {
		h.handleError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, user)
}

func (h *UserHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"message": "invalid user id"}})
		return
	}

	req, err := middleware.DecodeAndValidate(r, middleware.ValidateUpdateUser)
	if err != nil {
		var ve middleware.ValidationErrors
		if errors.As(err, &ve) {
			writeJSON(w, http.StatusBadRequest, map[string]any{"errors": ve})
			return
		}
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"message": err.Error()}})
		return
	}

	user, err := h.svc.UpdateUser(r.Context(), id, req.Name, req.Email)
	if err != nil {
		h.handleError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, user)
}

func (h *UserHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": map[string]string{"message": "invalid user id"}})
		return
	}

	if err := h.svc.DeleteUser(r.Context(), id); err != nil {
		h.handleError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	users, err := h.svc.GetAllUsers(r.Context())
	if err != nil {
		h.handleError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, users)
}

func (h *UserHandler) handleError(w http.ResponseWriter, err error) {
	if errors.Is(err, models.ErrUserNotFound) {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": map[string]string{"message": "User not found"}})
		return
	}

	h.log.Error("internal error", "error", err)
	writeJSON(w, http.StatusInternalServerError, map[string]any{"error": map[string]string{"message": "Internal Server Error"}})
}

func parseID(r *http.Request) (int, error) {
	return strconv.Atoi(r.PathValue("id"))
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
