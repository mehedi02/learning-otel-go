# syntax=docker/dockerfile:1.7

# ---- builder ---------------------------------------------------------------
# Pinned to the Go version declared in go.mod. Alpine for a smaller builder
# image; the final binary is statically linked so the build base doesn't
# affect the runtime base.
FROM golang:1.26-alpine AS builder

WORKDIR /src

# Pre-fetch modules in a separate layer so source-only changes don't bust the
# module cache.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

# Static, stripped binary. CGO_ENABLED=0 is required for the distroless static
# runtime image (no glibc available).
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build \
        -trimpath \
        -ldflags='-s -w' \
        -o /out/server ./cmd/server

# ---- runtime ---------------------------------------------------------------
# distroless/static is ~2 MB, no shell, no package manager, runs as non-root.
# Trade-off: zero ability to debug from inside the container; if you need
# `sh`, swap to `gcr.io/distroless/static:debug` temporarily.
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

COPY --from=builder /out/server /app/server
# Migrations are read with a relative path at startup; ship them with the binary.
COPY --from=builder /src/migrations /app/migrations

USER nonroot:nonroot
EXPOSE 5000

ENTRYPOINT ["/app/server"]
