# PROJECT.md — `learning-otel-go`

> **Purpose of this file:** complete context dump for any new chat / agent session.
> Read top-to-bottom and you'll know everything that was built, why each decision
> was made, the gotchas hit along the way, and the current state. Don't read this
> as a user-facing README — it's opinionated and assumes you'll act on it.

---

## 1. Origin & goals

- **Started from:** [Node.js OpenTelemetry tutorial](https://poridhi.io/launch/playground/regular/67588457fd07f8bfcaacb940/68f4a81f1696b9c20a138445/68f4a81f1696b9c20a138449)
  (saved at `F:\obsidian\my-research\Clippings\Intro to OpenTelemetry.md`).
  Reference codebase: sibling `../user-service/` directory (Node).
- **Goal:** port the Node user-service to Go *and* implement OpenTelemetry from
  scratch with Go-idiomatic, industry-best-practice patterns. Not a one-to-one
  copy — patterns adapted where Go conventions differ.
- **Pedagogy mode:** the assistant gives step guidance only; the user implements;
  the assistant audits each step before moving on. The user explicitly does **not**
  want full code dumps unless they're stuck.
- **Scope discipline:** the Node tutorial covers traces only. We went further and
  added metrics + log correlation + a containerized app. Loki was deliberately
  not added — log/trace correlation works via shared `trace_id` in JSON logs;
  shipping logs to Loki is a future ticket.

---

## 2. Tech stack & version pins

| Concern | Library / Image | Pinned | Notes |
|---|---|---|---|
| Go | `go 1.26` | go.mod | go.mod declares `go 1.26`. Builder image must match. |
| HTTP framework | `net/http` + `http.ServeMux` (1.22+) | stdlib | Method-prefixed patterns like `"POST /api/users/{id}"`. |
| Postgres driver | `github.com/jackc/pgx/v5` via `database/sql` (`stdlib`) | v5.9.2 | Blank import registers the `"pgx"` driver name. |
| Redis client | `github.com/redis/go-redis/v9` | v9.18.0 | |
| Config | `github.com/spf13/viper` | v1.21.0 | Reads `.env` + env vars; `mapstructure` tags map env keys to fields. |
| Logging | `log/slog` | stdlib | Decorated with custom `traceHandler`. |
| OTel SDK | `go.opentelemetry.io/otel` | v1.43.0 | All otel core packages on v1.43. |
| OTel HTTP | `.../contrib/.../otelhttp` | v0.68.0 | **Note:** `WithRouteTag` was removed in v0.59. We re-implemented it inline. |
| OTel SQL | `github.com/XSAM/otelsql` | v0.42.0 | Internally uses `semconv/v1.30.0`. |
| OTel Redis | `github.com/redis/go-redis/extra/redisotel/v9` | v9.19.0 | |
| OTel runtime | `.../contrib/instrumentation/runtime` | v0.68.0 | Go runtime metrics (goroutines/GC/mem). |
| OTel semconv (telemetry pkg) | `semconv/v1.40.0` | matches sdk v1.43 schema URL | Aligning resource semconv with `resource.Default()`'s schema URL is **mandatory** — see §7. |
| OTel semconv (db pkg) | `semconv/v1.30.0` | matches otelsql | Different file, different version is fine — only resources care about schema URL. |
| Tempo | `grafana/tempo:2.6.0` | compose | **Pinned**. Tempo's config schema breaks between minor versions. |
| Grafana | `grafana/grafana:11.3.0` | compose | |
| Prometheus | `prom/prometheus:v2.55.1` | compose | OTLP receiver requires 2.55+. |
| Postgres | `postgres:17` | compose | |
| Redis | `redis:7` | compose | |
| App runtime image | `gcr.io/distroless/static-debian12:nonroot` | Dockerfile | ~2 MB, no shell, UID 65532. |

---

## 3. Architecture (current state)

```
                          ┌────────────────────┐
                          │ Grafana (4000)     │
                          │  - Tempo DS        │
                          │  - Prometheus DS   │
                          └──┬─────────────┬───┘
                             │             │
              http://tempo:3200      http://prometheus:9090
                             │             │
                          ┌──▼──┐       ┌──▼─────────┐
                          │Tempo│       │ Prometheus │
                          └──▲──┘       └──▲─────────┘
                             │             │
                  OTLP/gRPC :4317     OTLP/HTTP :9090/api/v1/otlp/v1/metrics
                             │             │
                          ┌──┴─────────────┴──┐
                          │   user-service     │      ┌────────────┐
                          │   (Go app, :5000)  │─────▶│ Postgres   │
                          │                    │      │ :5432      │
                          │   - otelhttp       │      └────────────┘
                          │   - otelsql        │
                          │   - redisotel      │      ┌────────────┐
                          │   - manual spans   │─────▶│ Redis :6379│
                          │   - runtime metrics│      └────────────┘
                          │   - slog + trace_id│
                          └────────────────────┘
```

**Key invariants:**

- **Traces** flow over OTLP/gRPC to Tempo (`tempo:4317`).
- **Metrics** flow over OTLP/HTTP to Prometheus's built-in OTLP receiver
  (`prometheus:9090/api/v1/otlp/v1/metrics`). Prometheus is started with
  `--web.enable-otlp-receiver`.
- **Logs** stay on stdout as structured JSON; correlation is via the `trace_id`
  field injected by `traceHandler`. No log shipping yet.
- **Single network** `user-service` for all six containers.
- **Container DNS** is the source of truth for in-compose addressing
  (`tempo`, `prometheus`, `user-service-db`, `redis`). The host's `.env`
  uses `localhost:*` for "go run" mode — see §6 (two run modes).

---

## 4. File layout

```
learning-otel-go/
├── cmd/server/main.go                       # entrypoint: signal ctx + run() helper
├── internal/
│   ├── cache/
│   │   ├── cache.go                         # JSON wrapper around go-redis
│   │   └── redis.go                         # Client + redisotel tracing & metrics
│   ├── config/config.go                     # viper Config + applyDefaults + validate
│   ├── db/postgres.go                       # otelsql.Open + RegisterDBStatsMetrics + ExecContext migrations
│   ├── handler/user_handler.go              # 5 HTTP handlers, error translation
│   ├── logger/logger.go                     # slog + traceHandler decorator
│   ├── middleware/
│   │   ├── cors.go
│   │   ├── logging.go                       # AccessLog with InfoContext
│   │   ├── recovery.go                      # Records panics on the active span
│   │   ├── security.go
│   │   └── validate.go                      # Generic DecodeAndValidate[T]
│   ├── models/user.go                       # User struct + ErrUserNotFound sentinel
│   ├── repository/user_repository.go        # SQL CRUD via QueryRowContext / ExecContext
│   ├── router/router.go                     # otelhttp wraps stack; handle() helper for WithRouteTag
│   ├── service/user_service.go              # Manual spans (user.create/get/update/delete/list)
│   └── telemetry/telemetry.go               # SDK init (TracerProvider + MeterProvider)
├── migrations/001_users.sql                 # Loaded by RunMigrations at boot
├── tempo/tempo.yaml
├── prometheus/prometheus.yml
├── grafana/provisioning/datasources/datasource.yaml   # Tempo (default) + Prometheus
├── docker-compose.yaml                      # 6 services: redis, db, tempo, prometheus, grafana, app
├── Dockerfile                               # Multi-stage; builder + distroless static
├── .dockerignore
├── .env                                     # local dev (host mode), NOT committed if it has secrets
├── .env.example
├── go.mod / go.sum
└── PROJECT.md                               # this file
```

---

## 5. Step history (what was built, in order)

The whole project followed an 8-step roadmap. Each step was implemented by the
user, then audited and patched by the assistant.

### Step 1 — SDK foundation
- `internal/telemetry/telemetry.go` initializes a `TracerProvider` with:
  - Resource: `service.name`, `service.version`, `deployment.environment.name`
    (note the `.name` suffix — semconv v1.40 stabilization), `service.instance.id`
    (UUID per boot), merged with `resource.Default()`.
  - Exporter: OTLP/gRPC, no scheme in endpoint.
  - Processor: `BatchSpanProcessor` (never `Simple` outside tests).
  - Sampler: `ParentBased(TraceIDRatioBased(ratio))`, configurable.
  - Globals set: TracerProvider, composite propagator (`TraceContext` + `Baggage`),
    error handler routes SDK errors into slog.
- `cmd/server/main.go` was refactored to `main → run(ctx, log) error` with
  `signal.NotifyContext`. **Crucial:** telemetry shutdown is the FIRST `defer`
  registered so it runs LAST (LIFO), after redis/db close.

### Step 2 — Tempo + Grafana
- Tempo config: `http_listen_port: 3200` (the user originally wrote `5000` and
  spent debug cycles on that), distributor with both gRPC (4317) and HTTP (4318)
  receivers, local storage under `/var/tempo`, `block_retention: 24h`,
  `metrics_generator` deliberately removed.
- Grafana datasource provisioned with `uid: tempo`, anonymous Admin enabled.
- Single `user-service` network.

### Step 3 — HTTP auto-instrumentation
- `otelhttp.NewHandler` wraps the **outermost** handler so:
  - Span starts before any middleware runs (full-time capture).
  - Propagation extraction happens before middleware.
  - Downstream middleware can read the active span from `r.Context()`.
- Routes registered via `handle(mux, "POST /api/users", h.Create)` helper that
  splits the pattern at the first space and applies a local `withRoute(...)`
  function (re-implementation of the removed `otelhttp.WithRouteTag`):
  ```go
  func withRoute(route string, next http.Handler) http.Handler {
      return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
          span := trace.SpanFromContext(r.Context())
          span.SetAttributes(semconv.HTTPRoute(route))
          span.SetName(r.Method + " " + route)
          next.ServeHTTP(w, r)
      })
  }
  ```
- This is what keeps span names low-cardinality (`GET /api/users/{id}` instead
  of `GET /api/users/123`).

### Step 4 — Database auto-instrumentation
- `otelsql.Open("pgx", dsn, opts...)` replaces `sql.Open`.
- Static attributes via `WithAttributes`: `db.system.name=postgresql`,
  `db.namespace`, `server.address`, `server.port`.
- `WithSpanOptions` quiets noise: `OmitConnPrepare`, `OmitConnectorConnect`,
  `OmitConnResetSession`, `OmitRows`, `DisableErrSkip`. Keeps `OmitConnQuery=false`
  so actual SELECT/INSERT spans flow.
- `RunMigrations` now takes `ctx` and uses `ExecContext` (migration spans are
  child-of-ctx instead of orphans).
- Password redacted from the `connected to postgres` log line.
- Uses `semconv/v1.30.0` because that's what otelsql v0.42 uses internally.

### Step 5 — Redis auto-instrumentation
- `redisotel.InstrumentTracing(client)` after `redis.NewClient`, BEFORE the Ping,
  so even the startup ping produces a span.
- `redisotel.InstrumentMetrics(client)` added in Step 8.
- `_ = client.Close()` on the error path so failed instrumentation doesn't leak
  the pool.
- Cache hit/miss visibility comes via the `cache.hit` attribute on the `user.get`
  service span (Step 6) — not on the redis span itself.

### Step 6 — Manual service-layer spans
- Package tracer:
  ```go
  var tracer = otel.Tracer("github.com/mehedi/user-service-go/internal/service")
  ```
- All 5 service methods follow the same 5-element pattern:
  1. `ctx, span := tracer.Start(ctx, "user.<verb>", trace.WithAttributes(...))`
     **(re-bind ctx — the most common bug)**
  2. `defer span.End()` immediately
  3. Pass new `ctx` to all downstream calls (repo, cache)
  4. On real error: `span.RecordError(err)` + `span.SetStatus(codes.Error, ...)`
  5. Best-effort cache failures use `attribute.Bool("cache.write_error", true)`,
     NOT `SetStatus(Error)` — the operation succeeded.
- Span names: `user.create`, `user.get`, `user.update`, `user.delete`, `user.list`.
- Custom attributes namespaced under `app.*`: `app.user.id`, `app.user.count`.
  Reserved namespaces (`http.*`, `db.*`, etc.) are NEVER invented within.
- `cache.hit` boolean on `user.get` enables the cache-hit-rate TraceQL query.
- No PII in attributes (no email, no name as attributes — only structured logs
  capture them).

### Step 7 — Log/trace correlation
- `internal/logger/logger.go` defines `traceHandler` that embeds a `slog.Handler`:
  - `Handle`: pulls `SpanContextFromContext(ctx)`; if `IsValid()`, appends
    `trace_id` and `span_id` slog attributes.
  - `WithAttrs` / `WithGroup`: re-wrap so chained loggers don't lose enrichment.
- Switched in-request log calls to `*Context` variants:
  - All `s.log.Info(...)` / `s.log.Warn(...)` in `service/user_service.go` →
    `s.log.InfoContext(ctx, ...)` / `WarnContext`.
  - `middleware/logging.go` AccessLog → `log.InfoContext(r.Context(), ...)`.
  - `middleware/recovery.go` panic log → `log.ErrorContext(r.Context(), ...)`.
- Startup logs (`telemetry initialized`, `connected to postgres`, etc.)
  deliberately stay non-Context — no span exists at boot, no enrichment to do.
- **Bonus:** `recovery.go` now also calls `span.RecordError(err, trace.WithStackTrace(true))`
  + `span.SetStatus(codes.Error, "panic")` so panics show as red traces in
  Tempo with the message and stack.

### Step 8 — Metrics
- `internal/telemetry/telemetry.go` refactored to additionally configure a
  `MeterProvider` with an OTLP/HTTP exporter pointed at Prometheus's OTLP
  receiver path (`/api/v1/otlp/v1/metrics`), wrapped in a `PeriodicReader`
  (default 15s). Single `Init` returns ONE shutdown closure that calls
  `errors.Join(metricShutdown, traceShutdown)`.
- The SDK now manages the gRPC connection internally (no manual `conn.Close`).
- `cmd/server/main.go` calls `runtime.Start(...)` after `telemetry.Init` to push
  Go runtime metrics (goroutines, GC, mem, scheduler).
- `internal/db/postgres.go` calls `otelsql.RegisterDBStatsMetrics(db, ...)` for
  pool stats (open/idle/in-use, wait counts/durations). Note: the function returns
  `(metric.Registration, error)` — we discard the registration since process
  exit tears it down anyway.
- `internal/cache/redis.go` calls `redisotel.InstrumentMetrics(client)` for
  pool/command metrics.
- `otelhttp` HTTP server metrics flow automatically once the global
  `MeterProvider` is set — no extra wiring.
- `docker-compose.yaml` adds Prometheus pinned to v2.55.1 with
  `--web.enable-otlp-receiver` and `--enable-feature=otlp-write-receiver`.
- Grafana datasource adds Prometheus (UID `prometheus`); Tempo gets
  `tracesToMetrics.datasourceUid: prometheus` for the "View related metrics" link.

### Step 9 (bonus) — Containerization
- `Dockerfile`: multi-stage. Builder = `golang:1.26-alpine` with BuildKit cache
  mounts (`/go/pkg/mod`, `/root/.cache/go-build`). Runtime = distroless static
  (`gcr.io/distroless/static-debian12:nonroot`).
- Build flags: `CGO_ENABLED=0` (required for distroless static),
  `-trimpath -ldflags='-s -w'` (reproducible, smaller).
- `migrations/` copied in because `RunMigrations` reads via relative path.
- `.dockerignore` excludes VCS, infrastructure config, `.env`, editor metadata.
- `docker-compose.yaml` adds `app` service:
  - `build: .` + `image: user-service:local`
  - `restart: on-failure`
  - All env vars set in `environment:` block (NOT loaded from `.env`) —
    container DNS names differ from host names.
  - `depends_on` waits for db + redis to be `healthy` (added healthchecks via
    `pg_isready` and `redis-cli ping`).

---

## 6. How to run (two modes)

### Mode A — full compose (one command)
```powershell
docker compose up -d --build
docker compose ps                # 6 services
docker compose logs -f app
curl.exe http://localhost:5000/api/users
```
Rebuild after Go code change:
```powershell
docker compose up -d --build app
```

### Mode B — backends in compose, app on host (hot-reload dev)
```powershell
docker compose up -d redis user-service-db tempo prometheus grafana
go run .\cmd\server               # uses .env (localhost:* hosts)
```
Don't run both modes simultaneously — port 5000 collides.

### Endpoints
| Service | URL |
|---|---|
| App | http://localhost:5000/api/users |
| Grafana | http://localhost:4000 (anonymous Admin, no login) |
| Tempo HTTP | http://localhost:3200/ready |
| Prometheus | http://localhost:9090 |
| Redis | localhost:6400 (host-mapped from internal 6379) |
| Postgres | localhost:5432 (user/password/user-service) |

---

## 7. Hard-won lessons / gotchas

These bit during implementation. Write them on the wall.

1. **Schema URL collisions in `resource.Merge`.** `resource.Default()` from
   `sdk@v1.43` carries schema URL `1.40.0`. Importing `semconv/v1.26.0` in the
   same `resource.Merge` call returns
   `conflicting Schema URL: ... 1.40.0 and ... 1.26.0`. Fix: align the
   semconv import in the telemetry package with the SDK's schema URL. Other
   packages can use whatever semconv they want — only resources care.

2. **`semconv.DeploymentEnvironment` was renamed to `DeploymentEnvironmentName`**
   (and the attribute key changed from `deployment.environment` to
   `deployment.environment.name`) in semconv v1.27 stabilization. TraceQL
   queries must use the `.name` form.

3. **`otelhttp.WithRouteTag` was removed in `otelhttp v0.59.0`.** If you upgrade
   and your build breaks, the replacement is the 5-line `withRoute(...)` helper
   in `internal/router/router.go` — it sets `http.route` attribute and renames
   the span using `trace.SpanFromContext(r.Context())`.

4. **Re-binding `ctx` from `tracer.Start`.** `ctx, span := tracer.Start(ctx, ...)`
   returns a NEW ctx containing the new span. Every downstream call MUST receive
   the new ctx, otherwise child spans (DB/Redis) become children of the *parent*
   HTTP span and skip your domain span. If your trace tree shows the SQL span
   as a sibling of `user.get` instead of a child, this is why.

5. **Telemetry shutdown ordering.** Defer order matters:
   - `defer telemetryShutdown` (registered FIRST → runs LAST via LIFO)
   - `defer database.Close` (registered second → runs second-to-last)
   - `defer redisClient.Close` (registered third → runs first)
   - `srv.Shutdown(...)` runs INLINE before the function returns (defers run after).

6. **`signal.NotifyContext` vs `make(chan os.Signal)`.** Use `NotifyContext` —
   it gives you a ctx that's cancelled on signal, which is what you pass to
   `tracer.Start`, `srv.Shutdown`, etc. The return value `stop` must be
   `defer stop()`-ed to prevent signal-handler leaks.

7. **Listener-failure must short-circuit `run()`.** A goroutine running
   `srv.ListenAndServe()` should send any error to a buffered chan; the main
   `select` reads from `ctx.Done()` OR the err chan. Without this, a port-in-use
   error makes the process hang waiting for a signal that never comes.

8. **`grpc.NewClient` doesn't dial.** It only validates config; the connection
   is lazy. So "wrong endpoint" doesn't fail at startup — the BSP will silently
   buffer and retry. Don't write error messages like "connecting to ..." for
   that call; use "create otlp grpc client" instead.

9. **`OTEL_EXPORTER_OTLP_ENDPOINT` format.** OTLP gRPC exporters want bare
   `host:port`, NOT `http://host:port`. The `telemetry.normalizeEndpoint`
   helper strips the scheme defensively because users always paste URLs.

10. **Tempo `http_listen_port`** in `tempo.yaml` MUST match the port mapped
    in compose (3200). Setting it to anything else (e.g. accidentally 5000)
    makes Tempo unreachable from Grafana and from `curl localhost:3200/ready`.

11. **Tempo's `metrics_generator` block is a footgun in dev.** It tries to
    remote-write to Prometheus, needs Prometheus configured to accept
    remote-writes, and silently fails noisy. Skip it unless you actually need
    span-metrics or service graphs.

12. **Tempo image is distroless** — no `wget`, no `curl`, no shell. Don't add
    a Docker HEALTHCHECK using shell commands; it'll always be unhealthy.

13. **Prometheus 2.55+ OTLP endpoint** is `/api/v1/otlp/v1/metrics` (HTTP
    POST). Requires `--web.enable-otlp-receiver`. The exporter is
    `otlpmetrichttp`, not `otlpmetricgrpc` — Prometheus has no gRPC OTLP
    receiver.

14. **Default sampler ratio.** If the env var is missing and you don't set a
    default in `applyDefaults`, viper unmarshals `0.0` → `TraceIDRatioBased(0.0)`
    drops every root span. You spin up Tempo, hit endpoints, see nothing,
    and assume the exporter is broken. Default to `1.0` in `applyDefaults`.

15. **Two semconv versions in different files is fine.** The DB package uses
    `semconv/v1.30.0` (matches otelsql); the telemetry package uses
    `semconv/v1.40.0` (matches sdk Resource). Only `resource.Merge` cares about
    schema URL alignment; span attributes are free-form.

16. **Container DNS vs host names.** In compose, services reach each other at
    `tempo:4317`, `prometheus:9090`, `user-service-db:5432`, `redis:6379`.
    From the host, the same services are at `localhost:*` with possibly
    different ports (Redis is `6400` on the host, `6379` internally). The
    `app` service in compose has its OWN `environment:` block — don't be
    tempted to re-use the host `.env`.

17. **slog `*Context` variants.** `log.Info(...)` doesn't pass ctx to the
    handler, so `traceHandler.Handle` can't see the span. Use
    `log.InfoContext(ctx, ...)`. Easy to forget; symptom is a log line
    missing `trace_id` despite being mid-request.

18. **`WithAttrs`/`WithGroup` must wrap.** If `traceHandler.WithAttrs` returns
    the bare embedded handler, `log.With("k","v").InfoContext(ctx, ...)` loses
    trace correlation. Both methods must construct a new `traceHandler{...}`
    around the result.

19. **`semconv.DBSystemPostgreSQL` vs `DBSystemNamePostgreSQL`.** In semconv
    v1.30+, the constant is `DBSystemNamePostgreSQL` (the spec stabilized to
    `db.system.name`). TraceQL queries should use `db.system.name=postgresql`,
    not `db.system=postgresql`.

20. **`otelsql.RegisterDBStatsMetrics` returns `(metric.Registration, error)`**
    in v0.42. If you ignore both return values, build fails. Ignore the
    Registration with `_, err :=`.

---

## 8. Verification queries

After running with traffic (`for ($i=0; $i -lt 20; $i++) { curl.exe http://localhost:5000/api/users }`):

### TraceQL (Grafana → Explore → Tempo)
```
{ resource.service.name="user-service" }
{ resource.service.name="user-service" && name="POST /api/users" }
{ name="user.get" && cache.hit=true }
{ name="user.get" && cache.hit=false }
{ name="user.list" && app.user.count > 5 }
{ db.system.name="postgresql" }
{ status=error }
```

### PromQL (Grafana → Explore → Prometheus)
```
rate(http_server_request_duration_seconds_count[1m])
histogram_quantile(0.95, sum by (le, http_route) (rate(http_server_request_duration_seconds_bucket[5m])))
db_client_connections_usage
process_runtime_go_goroutines
process_runtime_go_mem_heap_alloc_bytes
```

### Log → trace handoff
1. `docker compose logs app | Select-String -Pattern "trace_id"`
2. Copy any `trace_id` value
3. Grafana → Explore → Tempo → "Search by Trace ID" → paste → land on the trace.

---

## 9. Open / future polish (not required, ranked by ROI)

1. **`/healthz` endpoint** + `otelhttp.WithFilter(...)` to exclude it from
   tracing. Useful before adding k8s probes.
2. **Embed migrations** with `//go:embed migrations/*.sql` so the runtime image
   doesn't need a `migrations/` directory. Then drop the COPY line from the
   Dockerfile.
3. **Production sampling.** Set `OTEL_TRACE_SAMPLE_RATIO=0.1` in prod config.
   `ParentBased` still honors upstream sampling decisions, so this only affects
   root sampling for requests without an inbound `traceparent`.
4. **Add Loki** + a slog handler that ships logs as OTel logs via
   `go.opentelemetry.io/contrib/bridges/otelslog`. The `trace_id` field already
   on every log line is what makes Tempo↔Loki "View related logs" work.
5. **Connection-pool config** in `internal/db/postgres.go` should be
   configurable, not hardcoded `25/5/5min`.
6. **Postgres + Redis env in compose** are duplicated between the `db`/`redis`
   service blocks and the `app` env. A `.env` file at the compose level would
   DRY this up — but explicitness is fine for a learning repo.
7. **Service/operation `code.*` attributes** auto-attached via a custom span
   processor would let Tempo display source file/line for every span. Nice for
   teams; not essential.
8. **Tempo `metrics_generator`** for service-graphs and span-metrics. Requires
   wiring Prometheus remote-write reception properly. Real value but adds
   moving parts.

---

## 10. Conventions cheat sheet

| Item | Convention |
|---|---|
| Span name | `domain.action`, lowercase, dot-separated. `user.create`, not `CreateUser`. |
| Tracer name | Full Go import path of the emitting package. |
| Custom attribute keys | `app.*` namespace. Never invent inside reserved namespaces (`http.*`, `db.*`, `messaging.*`, `network.*`, `code.*`, `cloud.*`, `k8s.*`, `host.*`, `process.*`, `service.*`, `client.*`, `server.*`). |
| Boolean attribute names | `cache.hit`, `cache.write_error` — adjective form. Not `cache_hit_yes`. |
| Span events | Reserve for things that happen DURING a span (retries, validations). NOT "started" / "completed" — `tracer.Start`/`span.End` already capture that. |
| Span status on success | Leave as `Unset` (default). Don't call `SetStatus(codes.Ok, ...)` defensively — it's reserved for "I'm explicitly overriding". |
| Span status on error | `RecordError(err)` AND `SetStatus(codes.Error, "<short reason>")`. Both, always. |
| `*Context` log variants | Use them inside request scope. Never inside startup. |
| Log fields containing PII | `email_domain` is OK; raw `email` is not (no deletion story for trace data). |
| Goroutine that owns a context | Owns shutdown of anything the ctx controls. Don't pass ctx to a goroutine and not handle its cancellation. |

---

## 11. Quick "I'm a new agent, what do I need to know" summary

- Go `user-service` ported from a Node tutorial, **fully instrumented** with
  OTel: traces (Tempo), metrics (Prometheus), log correlation (slog +
  `trace_id`/`span_id`).
- Six-container compose stack: app + redis + postgres + tempo + prometheus +
  grafana. Single `user-service` network. Pinned image versions.
- App can run two ways: in compose (`docker compose up -d --build`) or on the
  host (`go run ./cmd/server` against backend containers). Two distinct env
  setups — DON'T copy the host `.env` into the compose `app` service.
- Most important file: `internal/telemetry/telemetry.go` (single Init returns
  combined shutdown for both providers; SDK manages connections).
- Most surprising lesson: schema URL alignment in `resource.Merge` (§7.1).
- Project is feature-complete. Open items in §9 are polish, not blockers.
