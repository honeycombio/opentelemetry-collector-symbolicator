package proguardprocessor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processorhelper"
)

var (
	typeStr            = component.MustNewType("proguard_symbolicator")
	ErrorInvalidConfig = errors.New("invalid configuration for proguard processor")
)

func createDefaultConfig() component.Config {
	return &Config{
		SymbolicatorFailureAttributeKey: "exception.symbolication.failed",
		ClassesAttributeKey:             "exception.structured_stacktrace.classes",
		MethodsAttributeKey:             "exception.structured_stacktrace.methods",
		LinesAttributeKey:               "exception.structured_stacktrace.lines",
		OutputStackTraceKey:             "exception.stacktrace",
		StackTypeKey:                    "exception.type",
		StackMessageKey:                 "exception.message",
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

	symbolicator, err := newBasicSymbolicator(ctx, symCfg.Timeout, symCfg.ProguardCacheSize, store, set.Logger)
	if err != nil {
		return nil, err
	}

	processor, err := newProguardLogsProcessor(ctx, symCfg, store, set, symbolicator)

	if err != nil {
		return nil, err
	}

	return processorhelper.NewLogs(ctx, set, cfg, next, processor.ProcessLogs, processorhelper.WithCapabilities(consumer.Capabilities{MutatesData: true}))
}

func NewFactory() processor.Factory {
	return processor.NewFactory(
		typeStr,
		createDefaultConfig,
		processor.WithLogs(createLogsProcessor, component.StabilityLevelAlpha),
	)
}
