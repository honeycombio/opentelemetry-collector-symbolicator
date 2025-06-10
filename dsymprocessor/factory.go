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
	typeStr = component.MustNewType("symbolicator")
)

// createDefaultConfig creates the default configuration for the processor.
func createDefaultConfig() component.Config {
	return &Config{
		SymbolicatorFailureAttributeKey: "exception.symbolicator.failed",
		MetricKitStackTraceAttributeKey: "metrickit.diagnostic.crash.exception.stacktrace_json",
		OutputMetricKitStackTraceAttributeKey: "metrickit.diagnostic.crash.exception.stacktrace",
		PreserveStackTrace:              true,
		DSYMStoreKey:               "file_store",
		LocalSourceMapConfiguration: &LocalSourceMapConfiguration{
			Path: ".",
		},
		Timeout:            5 * time.Second,
		DSYMCacheSize: 128,
	}
}

// createTracesProcessor creates a traces processor
func createTracesProcessor(ctx context.Context, set processor.Settings, cfg component.Config, next consumer.Traces) (processor.Traces, error) {
	symCfg := cfg.(*Config)
	var store dsymStore
	var err error

	switch symCfg.DSYMStoreKey {
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

	sym, err := newBasicSymbolicator(ctx, symCfg.Timeout, symCfg.DSYMCacheSize, store)
	if err != nil {
		return nil, err
	}

	processor := newSymbolicatorProcessor(ctx, symCfg, set, sym)
	return processorhelper.NewTraces(ctx, set, cfg, next, processor.processTraces, processorhelper.WithCapabilities(consumer.Capabilities{MutatesData: true}))
}

// NewFactory creates a factory for the symbolicator processor
func NewFactory() processor.Factory {
	return processor.NewFactory(
		typeStr,
		createDefaultConfig,
		processor.WithTraces(createTracesProcessor, component.StabilityLevelAlpha),
	)
}
