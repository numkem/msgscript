package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.37.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// natsHeaderCarrier adapts NATS headers to OpenTelemetry propagation
type natsHeaderCarrier nats.Header

func (n natsHeaderCarrier) Get(key string) string {
	return nats.Header(n).Get(key)
}

func (n natsHeaderCarrier) Set(key string, value string) {
	nats.Header(n).Set(key, value)
}

func (n natsHeaderCarrier) Keys() []string {
	keys := make([]string, 0, len(n))
	for k := range n {
		keys = append(keys, k)
	}
	return keys
}

// setupOTelSDK bootstraps the OpenTelemetry pipeline.
// If it does not return an error, make sure to call shutdown for proper cleanup.
func setupOTelSDK(ctx context.Context) (func(context.Context) error, error) {
	var shutdownFuncs []func(context.Context) error
	var err error

	// shutdown calls cleanup functions registered via shutdownFuncs.
	// The errors from the calls are joined.
	// Each registered cleanup will be invoked once.
	shutdown := func(ctx context.Context) error {
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
	tracerProvider, err := newTracerProvider(ctx)
	if err != nil {
		handleErr(err)
		return shutdown, err
	}
	shutdownFuncs = append(shutdownFuncs, tracerProvider.Shutdown)
	otel.SetTracerProvider(tracerProvider)

	return shutdown, err
}

func newPropagator() propagation.TextMapPropagator {
	return propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	)
}

func tempoTraceExporter(ctx context.Context, tempoEndpoint string) (trace.SpanExporter, error) {
	conn, err := grpc.NewClient(
		tempoEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, err
	}

	return otlptracegrpc.New(ctx,
		otlptracegrpc.WithGRPCConn(conn),
	)
}

func newTracerProvider(ctx context.Context) (*trace.TracerProvider, error) {
	var traceExporter trace.SpanExporter
	var err error

	endpoint := os.Getenv("OTEL_ENDPOINT")
	if endpoint == "" {
		traceExporter, err = stdouttrace.New(
			stdouttrace.WithPrettyPrint())
		if err != nil {
			return nil, fmt.Errorf("failed to create stdout stdout trace exporter")
		}
	} else {
		traceExporter, err = tempoTraceExporter(ctx, endpoint)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to tempo GRPC endpoint: %v", err)
		}
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName("msgscript"),
			semconv.ServiceVersion(version),
		),
	)
	if err != nil {
		return nil, err
	}

	// Create tracer provider
	tracerProvider := trace.NewTracerProvider(
		trace.WithBatcher(traceExporter,
			trace.WithBatchTimeout(time.Second*5),
		),
		trace.WithResource(res),
	)

	return tracerProvider, nil
}
