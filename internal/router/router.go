package router

import (
	"log/slog"
	"net/http"

	"github.com/mehedi/user-service-go/internal/handler"
	"github.com/mehedi/user-service-go/internal/middleware"
)

func New(h *handler.UserHandler, log *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/users", h.Create)
	mux.HandleFunc("GET /api/users/{id}", h.GetByID)
	mux.HandleFunc("PUT /api/users/{id}", h.Update)
	mux.HandleFunc("DELETE /api/users/{id}", h.Delete)
	mux.HandleFunc("GET /api/users", h.List)

	stack := middleware.Chain(
		middleware.Recovery(log),
		middleware.AccessLog(log),
		middleware.CORS,
		middleware.Security,
	)

	return stack(mux)
}
