package dsymprocessor

import (
	"context"
	"time"

	"github.com/honeycombio/opentelemetry-collector-symbolicator/dsymprocessor/internal/metadata"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/processor/processorhelper"
	"go.opentelemetry.io/otel/attribute"
)

var (
	typeStr = component.MustNewType("dsym_symbolicator")
)

const (
	processorVersion = "1.0.2"
)

// createDefaultConfig creates the default configuration for the processor.
func createDefaultConfig() component.Config {
	return &Config{
		SymbolicatorFailureAttributeKey:             "exception.symbolicator.failed",
		SymbolicatorErrorAttributeKey:               "exception.symbolicator.error",
		StackTraceAttributeKey:                      "exception.stacktrace",
		MetricKitStackTraceAttributeKey:             "metrickit.diagnostic.crash.exception.stacktrace_json",
		OutputMetricKitStackTraceAttributeKey:       "exception.stacktrace",
		OutputMetricKitExceptionTypeAttributeKey:    "exception.type",
		OutputMetricKitExceptionMessageAttributeKey: "exception.message",
		PreserveStackTrace:                          true,
		OriginalStackTraceAttributeKey:              "exception.stacktrace.original",
		BuildUUIDAttributeKey:                       "app.debug.build_uuid",
		AppExecutableAttributeKey:                   "app.bundle.executable",
		DSYMStoreKey:                                "file_store",
		LocalDSYMConfiguration: &LocalDSYMConfiguration{
			Path: ".",
		},
		Timeout:              5 * time.Second,
		DSYMCacheSize:        128,
		LanguageAttributeKey: "telemetry.sdk.language",
		AllowedLanguages:     []string{}, // Empty by default, processes all signals
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

	tb, err := metadata.NewTelemetryBuilder(set.TelemetrySettings)
	if err != nil {
		return nil, err
	}
	// Set up resource attributes for telemetry
	attributeSet := setUpResourceAttributes()
	sym, err := newBasicSymbolicator(ctx, symCfg.Timeout, symCfg.DSYMCacheSize, store, tb, attributeSet)
	if err != nil {
		return nil, err
	}

	processor := newSymbolicatorProcessor(ctx, symCfg, set, sym, tb, attributeSet)
	return processorhelper.NewLogs(ctx, set, cfg, next, processor.processLogs, processorhelper.WithCapabilities(consumer.Capabilities{MutatesData: true}))
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
		processor.WithLogs(createLogsProcessor, component.StabilityLevelAlpha),
	)
}
