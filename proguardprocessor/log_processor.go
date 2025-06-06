package proguardprocessor

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.uber.org/zap"
)

var (
	errMissingAttribute = errors.New("missing attribute")
	errMismatchedLength = errors.New("mismatched stacktrace attribute lengths")
)

type proguardLogsProcessor struct {
	cfg    *Config
	logger *zap.Logger
}

func (p *proguardLogsProcessor) ProcessLogs(ctx context.Context, logs plog.Logs) (plog.Logs, error) {
	p.logger.Info("Processing logs")

	for i := 0; i < logs.ResourceLogs().Len(); i++ {
		rl := logs.ResourceLogs().At(i)

		if err := p.processResourceLogs(ctx, rl); err != nil {
			return logs, err
		}
	}

	return logs, nil
}

func (p *proguardLogsProcessor) processResourceLogs(ctx context.Context, rl plog.ResourceLogs) error {
	for j := 0; j < rl.ScopeLogs().Len(); j++ {
		sl := rl.ScopeLogs().At(j)
		if err := p.processScopeLogs(ctx, sl); err != nil {
			return err
		}
	}

	return nil
}

func (p *proguardLogsProcessor) processScopeLogs(ctx context.Context, sl plog.ScopeLogs) error {
	for k := 0; k < sl.LogRecords().Len(); k++ {
		lr := sl.LogRecords().At(k)
		if err := p.processLogRecord(ctx, lr); err != nil {
			return err
		}
	}

	return nil
}

func (p *proguardLogsProcessor) processLogRecord(ctx context.Context, lr plog.LogRecord) error {
	var ok bool
	var classes, methods, lines pcommon.Slice

	attrs := lr.Attributes()

	if classes, ok = getSlice(p.cfg.ClassesAttributeKey, attrs); !ok {
		return fmt.Errorf("%w: %s", errMissingAttribute, p.cfg.ClassesAttributeKey)
	}

	if methods, ok = getSlice(p.cfg.ClassesAttributeKey, attrs); !ok {
		return fmt.Errorf("%w: %s", errMissingAttribute, p.cfg.ClassesAttributeKey)
	}

	if lines, ok = getSlice(p.cfg.ClassesAttributeKey, attrs); !ok {
		return fmt.Errorf("%w: %s", errMissingAttribute, p.cfg.ClassesAttributeKey)
	}

	// Ensure all slices are the same length
	if classes.Len() != methods.Len() || classes.Len() != lines.Len() {
		return fmt.Errorf("%w: (%s %d) (%s %d) (%s %d) (%s %d)", errMismatchedLength,
			p.cfg.ClassesAttributeKey, classes.Len(),
			p.cfg.MethodsAttributeKey, methods.Len(),
			p.cfg.LinesAttributeKey, lines.Len(),
		)
	}

	p.logger.Debug("Processing log record", zap.String("body", lr.Body().AsString()))
}

func newProguardLogsProcessor(ctx context.Context, cfg *Config, logger *zap.Logger) (*proguardLogsProcessor, error) {
	return &proguardLogsProcessor{
		cfg:    cfg,
		logger: logger,
	}, nil
}

// getSlice retrieves a slice from a map, returning an empty slice if the key is not found.
func getSlice(key string, m pcommon.Map) (pcommon.Slice, bool) {
	v, ok := m.Get(key)
	if !ok {
		return pcommon.NewSlice(), false
	}

	return v.Slice(), true
}
