package dsymprocessor

import (
	"context"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processorhelper"
)

var (
	typeStr = component.MustNewType("dsym_symbolicator")
)

const (
	processorVersion = "0.0.6"
)

// createDefaultConfig creates the default configuration for the processor.
func createDefaultConfig() component.Config {
	return &Config{
		SymbolicatorFailureAttributeKey:             "exception.symbolicator.failed",
		SymbolicatorFailureMessageAttributeKey:      "exception.symbolicator.error",
		StackTraceAttributeKey:                      "exception.stacktrace",
		OriginalStackTraceKey:                       "exception.stacktrace.original",
		AppExecutableAttributeKey:                   "app.bundle.executable",
		BuildUUIDAttributeKey:                       "app.debug.build_uuid",
		MetricKitStackTraceAttributeKey:             "metrickit.diagnostic.crash.exception.stacktrace_json",
		OutputMetricKitStackTraceAttributeKey:       "exception.stacktrace",
		OutputMetricKitExceptionTypeAttributeKey:    "exception.type",
		OutputMetricKitExceptionMessageAttributeKey: "exception.message",
		PreserveStackTrace:                          true,
		DSYMStoreKey:                                "file_store",
		LocalDSYMConfiguration: &LocalDSYMConfiguration{
			Path: ".",
		},
		Timeout:       5 * time.Second,
		DSYMCacheSize: 128,
	}
}

// createLogsProcessor creates a logs processor
func createLogsProcessor(ctx context.Context, set processor.Settings, cfg component.Config, next consumer.Logs) (processor.Logs, error) {
	symCfg := cfg.(*Config)
	var store dsymStore
	var err error

	switch symCfg.DSYMStoreKey {
	case "file_store":
		store, err = newFileStore(ctx, set.Logger, symCfg.LocalDSYMConfiguration)
	case "s3_store":
		store, err = newS3Store(ctx, set.Logger, symCfg.S3DSYMConfiguration)
	case "gcs_store":
		store, err = newGCSStore(ctx, set.Logger, symCfg.GCSDSYMConfiguration)
	}

	if err != nil {
		return nil, err
	}

	sym, err := newBasicSymbolicator(ctx, symCfg.Timeout, symCfg.DSYMCacheSize, store)
	if err != nil {
		return nil, err
	}

	processor := newSymbolicatorProcessor(ctx, symCfg, set, sym)
	return processorhelper.NewLogs(ctx, set, cfg, next, processor.processLogs, processorhelper.WithCapabilities(consumer.Capabilities{MutatesData: true}))
}

// NewFactory creates a factory for the symbolicator processor
func NewFactory() processor.Factory {
	return processor.NewFactory(
		typeStr,
		createDefaultConfig,
		processor.WithLogs(createLogsProcessor, component.StabilityLevelAlpha),
	)
}
