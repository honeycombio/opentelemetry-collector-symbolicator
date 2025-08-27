package proguardprocessor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/honeycombio/opentelemetry-collector-symbolicator/proguardprocessor/internal/metadata"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processorhelper"
	"go.opentelemetry.io/otel/attribute"
)

var (
	typeStr            = component.MustNewType("proguard_symbolicator")
	ErrorInvalidConfig = errors.New("invalid configuration for proguard processor")
)

const (
	processorVersion = "0.0.4"
)

func createDefaultConfig() component.Config {
	return &Config{
		SymbolicatorFailureAttributeKey: "exception.symbolicator.failed",
		SymbolicatorErrorAttributeKey:   "exception.symbolicator.error",
		ClassesAttributeKey:             "exception.structured_stacktrace.classes",
		MethodsAttributeKey:             "exception.structured_stacktrace.methods",
		LinesAttributeKey:               "exception.structured_stacktrace.lines",
		OutputStackTraceKey:             "exception.stacktrace",
		ExceptionTypeAttributeKey:       "exception.type",
		ExceptionMessageAttributeKey:    "exception.message",
		PreserveStackTrace:              true,
		OriginalClassesAttributeKey:     "exception.structured_stacktrace.classes.original",
		OriginalMethodsAttributeKey:     "exception.structured_stacktrace.methods.original",
		OriginalLinesAttributeKey:       "exception.structured_stacktrace.lines.original",
		OriginalStackTraceKey:           "exception.stacktrace.original",
		ProguardUUIDAttributeKey:        "app.debug.proguard_uuid",
		ProguardStoreKey:                "file_store",
		LocalProguardConfiguration: &LocalStoreConfiguration{
			Path: ".",
		},
		Timeout:           5 * time.Second,
		ProguardCacheSize: 128,
	}
}

func createLogsProcessor(ctx context.Context, set processor.Settings, cfg component.Config, next consumer.Logs) (processor.Logs, error) {
	symCfg, ok := cfg.(*Config)

	if !ok {
		return nil, fmt.Errorf("%w: expected Config type, got %T", ErrorInvalidConfig, cfg)
	}

	var store fileStore
	var err error

	switch symCfg.ProguardStoreKey {
	case "file_store":
		store, err = newFileStore(ctx, set.Logger, symCfg.LocalProguardConfiguration)
	case "s3_store":
		store, err = newS3Store(ctx, set.Logger, symCfg.S3ProguardConfiguration)
	case "gcs_store":
		store, err = newGCSStore(ctx, set.Logger, symCfg.GCSProguardConfiguration)
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
	symbolicator, err := newBasicSymbolicator(ctx, symCfg.Timeout, symCfg.ProguardCacheSize, store, tb, attributeSet)
	if err != nil {
		return nil, err
	}

	processor, err := newProguardLogsProcessor(ctx, symCfg, store, set, symbolicator, tb, attributeSet)

	if err != nil {
		return nil, err
	}

	return processorhelper.NewLogs(ctx, set, cfg, next, processor.ProcessLogs, processorhelper.WithCapabilities(consumer.Capabilities{MutatesData: true}))
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

func NewFactory() processor.Factory {
	return processor.NewFactory(
		typeStr,
		createDefaultConfig,
		processor.WithLogs(createLogsProcessor, component.StabilityLevelAlpha),
	)
}
