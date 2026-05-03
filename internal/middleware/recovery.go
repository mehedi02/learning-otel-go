package middleware

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"

	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

func Recovery(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				rec := recover()
				if rec == nil {
					return
				}

				// Record on the active span (set by otelhttp) so panics show
				// as red traces in Tempo with the panic message and stack.
				err := fmt.Errorf("panic: %v", rec)
				span := trace.SpanFromContext(r.Context())
				span.RecordError(err, trace.WithStackTrace(true))
				span.SetStatus(codes.Error, "panic")

				log.ErrorContext(r.Context(), "panic recovered",
					"error", rec,
					"method", r.Method,
					"path", r.URL.Path,
					"stack", string(debug.Stack()),
				)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"error": map[string]string{
						"message": "Internal Server Error",
					},
				})
			}()
			next.ServeHTTP(w, r)
		})
	}
}
