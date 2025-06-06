package proguardprocessor

import (
	"context"
	"errors"

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
	return &Config{}
}

func createLogsProcessor(ctx context.Context, set processor.Settings, cfg component.Config, next consumer.Logs) (processor.Logs, error) {
	symCfg, ok := cfg.(*Config)

	if !ok {
		return nil, ErrorInvalidConfig
	}
	processor, err := newProguardLogsProcessor(ctx, symCfg, set.Logger)

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
