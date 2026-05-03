package router

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/mehedi/user-service-go/internal/handler"
	"github.com/mehedi/user-service-go/internal/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
)

func New(h *handler.UserHandler, log *slog.Logger) http.Handler {
	mux := http.NewServeMux()

	handle(mux, "POST /api/users", h.Create)
	handle(mux, "GET /api/users/{id}", h.GetByID)
	handle(mux, "PUT /api/users/{id}", h.Update)
	handle(mux, "DELETE /api/users/{id}", h.Delete)
	handle(mux, "GET /api/users", h.List)

	stack := middleware.Chain(
		middleware.Recovery(log),
		middleware.AccessLog(log),
		middleware.CORS,
		middleware.Security,
	)

	// otelhttp is the OUTERMOST handler so the root span starts as soon as
	// the request arrives (capturing all middleware time) and downstream
	// middleware can pull the active span out of r.Context() for log
	// correlation.
	return otelhttp.NewHandler(stack(mux), "http.server")
}

// handle registers an instrumented route. The pattern is "METHOD /route"; the
// route portion is extracted and used as the http.route attribute and span
// name template, keeping span cardinality bounded by route count rather than
// path-parameter values. This replaces the removed otelhttp.WithRouteTag
// helper (gone since otelhttp v0.59).
func handle(mux *http.ServeMux, pattern string, h http.HandlerFunc) {
	route := pattern
	if i := strings.IndexByte(pattern, ' '); i >= 0 {
		route = pattern[i+1:]
	}
	mux.Handle(pattern, withRoute(route, h))
}

func withRoute(route string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		span := trace.SpanFromContext(r.Context())
		span.SetAttributes(semconv.HTTPRoute(route))
		span.SetName(r.Method + " " + route)
		next.ServeHTTP(w, r)
	})
}
