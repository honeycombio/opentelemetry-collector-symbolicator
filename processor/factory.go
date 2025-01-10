package symbolicatorprocessor

import (
	"context"

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
		ColumnsAttributeKey:           "exception.structured_stacktrace.columns",
		FunctionsAttributeKey:         "exception.structured_stacktrace.functions",
		LinesAttributeKey:             "exception.structured_stacktrace.lines",
		UrlsAttributeKey:              "exception.structured_stacktrace.urls",
		OutputStackTraceKey:           "exception.stacktrace",
		StackTypeKey:                  "exception.type",
		StackMessageKey:               "exception.message",
		PreserveStackTrace:            true,
		OriginalStackTraceKey:         "exception.stacktrace.original",
		OriginalFunctionsAttributeKey: "exception.structured_stacktrace.functions.original",
		OriginalLinesAttributeKey:     "exception.structured_stacktrace.lines.original",
		OriginalColumnsAttributeKey:   "exception.structured_stacktrace.columns.original",
		OriginalUrlsAttributeKey:      "exception.structured_stacktrace.urls.original",
		SourceMapFilePath:             ".",
	}
}

// createTracesProcessor creates a traces processor
func createTracesProcessor(ctx context.Context, set processor.Settings, cfg component.Config, next consumer.Traces) (processor.Traces, error) {
	symCfg := cfg.(*Config)
	fs := newFileStore(symCfg.SourceMapFilePath, set.Logger)
	sym := newBasicSymbolicator(fs)
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
