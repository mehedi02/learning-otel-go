package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
)

// Config holds everything needed to initialize tracing AND metrics.
type Config struct {
	ServiceName     string
	ServiceVersion  string
	Environment     string
	ResourceExtra   []string // optional extra k=v attributes (currently unused)

	// Traces (OTLP/gRPC, Tempo).
	TraceEndpoint    string  // host:port
	TraceSampleRatio float64

	// Metrics (OTLP/HTTP, Prometheus's OTLP receiver).
	MetricsEndpoint       string        // host:port; URL path is fixed below
	MetricsExportInterval time.Duration // PeriodicReader interval; default 15s
}

// Init configures the global TracerProvider, MeterProvider, propagator, and
// error handler. The returned shutdown closure flushes both providers (and
// closes their exporters) with a 5s deadline. Caller must invoke it before
// process exit.
func Init(ctx context.Context, cfg Config, log *slog.Logger) (shutdown func(context.Context) error, err error) {
	res, err := buildResource(cfg)
	if err != nil {
		return nil, fmt.Errorf("telemetry: build resource: %w", err)
	}

	traceEndpoint := normalizeEndpoint(cfg.TraceEndpoint)
	tp, traceShutdown, err := initTracerProvider(ctx, cfg, traceEndpoint, res)
	if err != nil {
		return nil, fmt.Errorf("telemetry: tracer provider: %w", err)
	}

	metricsEndpoint := normalizeEndpoint(cfg.MetricsEndpoint)
	mp, metricShutdown, err := initMeterProvider(ctx, cfg, metricsEndpoint, res)
	if err != nil {
		_ = traceShutdown(ctx)
		return nil, fmt.Errorf("telemetry: meter provider: %w", err)
	}

	otel.SetTracerProvider(tp)
	otel.SetMeterProvider(mp)
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		log.Error("otel sdk error", "error", err)
	}))

	log.Info("telemetry initialized",
		"service.name", cfg.ServiceName,
		"service.version", cfg.ServiceVersion,
		"deployment.environment", cfg.Environment,
		"otlp.trace.endpoint", traceEndpoint,
		"otlp.metrics.endpoint", metricsEndpoint,
		"sample.ratio", cfg.TraceSampleRatio,
	)

	shutdown = func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		// Shut metrics down first, then traces, so any spans the metrics
		// shutdown happens to emit can still be flushed.
		return errors.Join(metricShutdown(ctx), traceShutdown(ctx))
	}

	return shutdown, nil
}

func buildResource(cfg Config) (*resource.Resource, error) {
	return resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
			semconv.DeploymentEnvironmentName(cfg.Environment),
			semconv.ServiceInstanceID(uuid.NewString()),
		),
	)
}

func initTracerProvider(
	ctx context.Context,
	cfg Config,
	endpoint string,
	res *resource.Resource,
) (*sdktrace.TracerProvider, func(context.Context) error, error) {
	exporter, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create trace exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithSpanProcessor(sdktrace.NewBatchSpanProcessor(exporter)),
		sdktrace.WithSampler(
			sdktrace.ParentBased(sdktrace.TraceIDRatioBased(cfg.TraceSampleRatio)),
		),
	)
	return tp, tp.Shutdown, nil
}

func initMeterProvider(
	ctx context.Context,
	cfg Config,
	endpoint string,
	res *resource.Resource,
) (*sdkmetric.MeterProvider, func(context.Context) error, error) {
	// Prometheus 2.55+ exposes an OTLP/HTTP receiver at this fixed path
	// when started with --web.enable-otlp-receiver.
	exporter, err := otlpmetrichttp.New(ctx,
		otlpmetrichttp.WithEndpoint(endpoint),
		otlpmetrichttp.WithURLPath("/api/v1/otlp/v1/metrics"),
		otlpmetrichttp.WithInsecure(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("create metric exporter: %w", err)
	}

	interval := cfg.MetricsExportInterval
	if interval <= 0 {
		interval = 15 * time.Second
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(
			sdkmetric.NewPeriodicReader(exporter, sdkmetric.WithInterval(interval)),
		),
	)
	return mp, mp.Shutdown, nil
}

// normalizeEndpoint strips http:// or https:// prefixes so users can paste
// either a URL or a bare host:port into their .env without breaking the
// gRPC/HTTP exporters (both expect host:port).
func normalizeEndpoint(ep string) string {
	ep = strings.TrimPrefix(ep, "https://")
	ep = strings.TrimPrefix(ep, "http://")
	return ep
}
