package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"strings"
	"time"

	"backend/internal/endpoint"
	"backend/internal/middleware"

	"github.com/XSAM/otelsql"
	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/valkey-io/valkey-go"
	"github.com/valkey-io/valkey-go/valkeyotel"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploggrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	oteltrace "go.opentelemetry.io/otel/trace"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	slog.Info("setting up OpenTelemetry SDK")
	otelhttp.DefaultClient.Transport = otelhttp.NewTransport(transport{})
	otelShutdown, err := setupOTelSDK(context.Background())
	if err != nil {
		slog.Error("Failed to setup OpenTelemetry SDK", slog.Any("error", err))
		return
	}

	dataSourceName := os.Getenv("DDFEED_BACKEND_DATA_SOURCE_NAME")
	if dataSourceName == "" {
		slog.Error("DDFEED_BACKEND_DATA_SOURCE_NAME is required")
		return
	}
	var db *sql.DB
	var dbx *sqlx.DB
	var dbConnectionError error
	for i := range 10 {
		db, dbConnectionError = otelsql.Open("mysql", dataSourceName, otelsql.WithAttributes(semconv.DBSystemMySQL))
		if dbConnectionError != nil {
			slog.Debug("Failed to connect to database", slog.Any("error", dbConnectionError))
			time.Sleep(time.Second * time.Duration(i))
			continue
		}
		if db != nil {
			dbx = sqlx.NewDb(db, "mysql")
			break
		}
	}
	if db == nil {
		slog.Error("Failed to connect to database", slog.Any("error", dbConnectionError))
		return
	}
	defer db.Close()
	if dbx == nil {
		slog.Error("Failed to create sqlx database", slog.Any("error", dbConnectionError))
		return
	}
	defer dbx.Close()

	vk, err := valkeyotel.NewClient(valkey.ClientOption{
		InitAddress: []string{"valkey:6379"},
	})
	if err != nil {
		slog.Error("Failed to create Valkey client", slog.Any("error", err))
		return
	}

	endpoint.Register(func(pattern string, handler func(http.ResponseWriter, *http.Request)) {
		route := pattern
		parts := strings.Split(pattern, " ")
		if len(parts) == 2 {
			// Trim HTTP method.
			// GET /v1/posts -> /v1/posts
			// Datadog Resource Name: HTTP method + route
			route = parts[1]
		}
		http.Handle(
			pattern,
			otelhttp.NewHandler(
				otelhttp.WithRouteTag(
					route,
					http.HandlerFunc(handler),
				),
				pattern,
			),
		)
	}, dbx, vk)

	port := os.Getenv("DDFEED_BACKEND_PORT")
	if port == "" {
		port = "8080"
	}
	slog.Info("Starting server on port " + port)
	go func() {
		if err := http.ListenAndServe(":"+port, middleware.Wrap(http.DefaultServeMux, middleware.CORS())); err != nil {
			slog.Error("Failed to start server", slog.Any("error", err))
		}
	}()

	<-ctx.Done()
	slog.Info("Server stopped")
	otelShutdown(context.Background())
}

type transport struct{}

func (transport) RoundTrip(r *http.Request) (*http.Response, error) {
	ctx := r.Context()
	s := oteltrace.SpanFromContext(ctx)
	resource := fmt.Sprintf("%s %s", r.Method, r.URL.Path)
	s.SetAttributes(attribute.String("resource.name", resource))
	r = r.WithContext(oteltrace.ContextWithSpan(ctx, s))
	return http.DefaultTransport.RoundTrip(r)
}

// setupOTelSDK bootstraps the OpenTelemetry pipeline.
// If it does not return an error, make sure to call shutdown for proper cleanup.
func setupOTelSDK(ctx context.Context) (shutdown func(context.Context) error, err error) {
	var shutdownFuncs []func(context.Context) error

	// shutdown calls cleanup functions registered via shutdownFuncs.
	// The errors from the calls are joined.
	// Each registered cleanup will be invoked once.
	shutdown = func(ctx context.Context) error {
		var err error
		for _, fn := range shutdownFuncs {
			err = errors.Join(err, fn(ctx))
		}
		shutdownFuncs = nil
		return err
	}

	// handleErr calls shutdown for cleanup and makes sure that all errors are returned.
	handleErr := func(inErr error) {
		err = errors.Join(inErr, shutdown(ctx))
	}

	// Set up propagator.
	prop := newPropagator()
	otel.SetTextMapPropagator(prop)

	// Set up trace provider.
	tracerProvider, err := newTraceProvider()
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	// Set up meter provider.
	meterProvider, err := newMeterProvider()
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, meterProvider.Shutdown)
	otel.SetMeterProvider(meterProvider)

	// Start go runtime metric collection.
	if err := runtime.Start(runtime.WithMinimumReadMemStatsInterval(time.Second)); err != nil {
		slog.Error("failed to start runtime metric collection", slog.Any("error", err))
	}

	// Set up logger provider.
	loggerProvider, err := newLoggerProvider()
	if err != nil {
		handleErr(err)
		return
	}
	shutdownFuncs = append(shutdownFuncs, loggerProvider.Shutdown)
	global.SetLoggerProvider(loggerProvider)

	return
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func newTraceProvider() (*trace.TracerProvider, error) {
	var opts []trace.TracerProviderOption
	traceExporter, err := otlptracegrpc.New(context.Background(),
		otlptracegrpc.WithInsecure(),
		otlptracegrpc.WithEndpoint(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")),
	)
	if err != nil {
		return nil, fmt.Errorf("creating grpc trace exporter: %w", err)
	}
	opts = append(opts, trace.WithSyncer(traceExporter))

	if v := os.Getenv("OTEL_TRACE_DEBUG"); v == "true" {
		slog.Info("debug tracing enabled, adding stdout printer")
		stdoutPrinter, err := stdouttrace.New(stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, fmt.Errorf("creating stdout printer: %w", err)
		}
		opts = append(opts, trace.WithSyncer(stdoutPrinter))
	}
	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
		),
	)
	if err != nil {
		return nil, fmt.Errorf("merging resource: %w", err)
	}
	opts = append(opts, trace.WithResource(r))
	slog.Info("trace provider created successfully")
	return trace.NewTracerProvider(opts...), nil
}

func newMeterProvider() (*metric.MeterProvider, error) {
	metricExporter, err := otlpmetricgrpc.New(context.Background(),
		otlpmetricgrpc.WithInsecure(),
		otlpmetricgrpc.WithEndpoint(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create metric exporter: %w", err)
	}
	r, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
		),
	)
	if err != nil {
		slog.Warn("failed to merge resource, using default resource", slog.Any("error", err))
		r = resource.Default()
	}
	meterProvider := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(metricExporter, metric.WithInterval(3*time.Second))),
		metric.WithResource(r),
	)
	slog.Info("meter provider created successfully")
	return meterProvider, nil
}

func newLoggerProvider() (*log.LoggerProvider, error) {
	logExporter, err := otlploggrpc.New(context.Background(),
		otlploggrpc.WithInsecure(),
		otlploggrpc.WithEndpoint(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create log exporter: %w", err)
	}

	loggerProvider := log.NewLoggerProvider(
		log.WithProcessor(log.NewBatchProcessor(logExporter)),
	)
	slog.Info("logger provider created successfully")
	return loggerProvider, nil
}
