package sourcemapprocessor

import (
	"context"
	"time"

	"github.com/honeycombio/opentelemetry-collector-symbolicator/sourcemapprocessor/internal/metadata"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processorhelper"
	"go.opentelemetry.io/otel/attribute"
)

var (
	typeStr = component.MustNewType("source_map_symbolicator")
)

const (
	processorVersion = "0.0.13"
)

// createDefaultConfig creates the default configuration for the processor.
func createDefaultConfig() component.Config {
	return &Config{
		SymbolicatorFailureAttributeKey:        "exception.symbolicator.failed",
		SymbolicatorFailureMessageAttributeKey: "exception.symbolicator.error",
		ColumnsAttributeKey:                    "exception.structured_stacktrace.columns",
		FunctionsAttributeKey:                  "exception.structured_stacktrace.functions",
		LinesAttributeKey:                      "exception.structured_stacktrace.lines",
		UrlsAttributeKey:                       "exception.structured_stacktrace.urls",
		OutputStackTraceKey:                    "exception.stacktrace",
		StackTypeKey:                           "exception.type",
		StackMessageKey:                        "exception.message",
		PreserveStackTrace:                     true,
		OriginalStackTraceKey:                  "exception.stacktrace.original",
		OriginalFunctionsAttributeKey:          "exception.structured_stacktrace.functions.original",
		OriginalLinesAttributeKey:              "exception.structured_stacktrace.lines.original",
		OriginalColumnsAttributeKey:            "exception.structured_stacktrace.columns.original",
		OriginalUrlsAttributeKey:               "exception.structured_stacktrace.urls.original",
		BuildUUIDAttributeKey:                  "app.debug.source_map_uuid",
		SourceMapStoreKey:                      "file_store",
		LocalSourceMapConfiguration: &LocalSourceMapConfiguration{
			Path: ".",
		},
		Timeout:            5 * time.Second,
		SourceMapCacheSize: 128,
	}
}

// createSymbolicatorProcessor is a helper that creates the common symbolicator processor
// used by both traces and logs processors.
func createSymbolicatorProcessor(ctx context.Context, set processor.Settings, cfg component.Config) (*symbolicatorProcessor, error) {
	symCfg := cfg.(*Config)
	var store sourceMapStore
	var err error

	switch symCfg.SourceMapStoreKey {
	case "file_store":
		store, err = newFileStore(ctx, set.Logger, symCfg.LocalSourceMapConfiguration)
	case "s3_store":
		store, err = newS3Store(ctx, set.Logger, symCfg.S3SourceMapConfiguration)
	case "gcs_store":
		store, err = newGCSStore(ctx, set.Logger, symCfg.GCSSourceMapConfiguration)
	}

	if err != nil {
		return nil, err
	}

	tb, err := metadata.NewTelemetryBuilder(set.TelemetrySettings)
	if err != nil {
		return nil, err
	}

	// Set up resource attributes for telemetry
	attributeSet := setUpResourceAttributes()
	sym, err := newBasicSymbolicator(ctx, symCfg.Timeout, symCfg.SourceMapCacheSize, store, tb, attributeSet)
	if err != nil {
		return nil, err
	}

	return newSymbolicatorProcessor(ctx, symCfg, set, sym, tb, attributeSet), nil
}

// createTracesProcessor creates a processor that accepts traces.
func createTracesProcessor(ctx context.Context, set processor.Settings, cfg component.Config, next consumer.Traces) (processor.Traces, error) {
	proc, err := createSymbolicatorProcessor(ctx, set, cfg)
	if err != nil {
		return nil, err
	}
	return processorhelper.NewTraces(ctx, set, cfg, next, proc.processTraces, processorhelper.WithCapabilities(consumer.Capabilities{MutatesData: true}))
}

// createLogsProcessor creates a processor that accepts logs.
func createLogsProcessor(ctx context.Context, set processor.Settings, cfg component.Config, next consumer.Logs) (processor.Logs, error) {
	proc, err := createSymbolicatorProcessor(ctx, set, cfg)
	if err != nil {
		return nil, err
	}
	return processorhelper.NewLogs(ctx, set, cfg, next, proc.processLogs, processorhelper.WithCapabilities(consumer.Capabilities{MutatesData: true}))
}

func setUpResourceAttributes() attribute.Set {
	attributes := []attribute.KeyValue{}
	config := metadata.DefaultResourceAttributesConfig()

	if config.ProcessorType.Enabled {
		attributes = append(attributes, attribute.String("otelcol_processor_type", typeStr.String()))
	}
	if config.ProcessorVersion.Enabled {
		attributes = append(attributes, attribute.String("otelcol_processor_version", processorVersion))
	}

	return attribute.NewSet(attributes...)
}

// NewFactory creates a factory for the symbolicator processor
func NewFactory() processor.Factory {
	return processor.NewFactory(
		typeStr,
		createDefaultConfig,
		processor.WithTraces(createTracesProcessor, component.StabilityLevelAlpha),
		processor.WithLogs(createLogsProcessor, component.StabilityLevelAlpha),
	)
}
