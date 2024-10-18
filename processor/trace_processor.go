package symbolicatorprocessor

import (
	"context"

	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

type symbolicatorProcessor struct {
	logger *zap.Logger

	cfg *Config
}

func newSymbolicatorProcessor(cfg *Config, set processor.Settings) *symbolicatorProcessor {
	set.Logger.Info("Creating new symbolicator processor")
	return &symbolicatorProcessor{
		cfg:    cfg,
		logger: set.Logger,
	}
}

func (sp *symbolicatorProcessor) processTraces(ctx context.Context, td ptrace.Traces) (ptrace.Traces, error) {
	sp.logger.Info("Symbolicator processor is called")
	return td, nil
}
