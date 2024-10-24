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

func createDefaultConfig() component.Config {
	return &Config{}
}

func createTracesProcessor(ctx context.Context, set processor.Settings, cfg component.Config, next consumer.Traces) (processor.Traces, error) {
	symCfg := cfg.(*Config)
	processor := newSymbolicatorProcessor(symCfg, set)
	return processorhelper.NewTraces(ctx, set, cfg, next, processor.processTraces, processorhelper.WithCapabilities(consumer.Capabilities{MutatesData: true}))
}

func NewFactory() processor.Factory {
	return processor.NewFactory(
		typeStr,
		createDefaultConfig,
		processor.WithTraces(createTracesProcessor, component.StabilityLevelAlpha),
	)
}
