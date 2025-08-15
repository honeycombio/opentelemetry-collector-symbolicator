package proguardprocessor

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

var (
	errMissingAttribute = errors.New("missing attribute")
	errMismatchedLength = errors.New("mismatched stacktrace attribute lengths")
	errPartialSymbolication = errors.New("symbolication failed for some stack frames")
)

// symbolicator interface is used to symbolicate stack traces.
type symbolicator interface {
	symbolicate(ctx context.Context, uuid, class, method string, line int) ([]*mappedStackFrame, error)
}

type proguardLogsProcessor struct {
	cfg          *Config
	logger       *zap.Logger
	symbolicator symbolicator
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
	attributes := lr.Attributes()

	err := p.processLogRecordThrow(ctx, attributes)

	if err != nil {
		attributes.PutBool(p.cfg.SymbolicatorFailureAttributeKey, true)
		return err
	} else {
		attributes.PutBool(p.cfg.SymbolicatorFailureAttributeKey, false)
		return nil
	}
}

func (p *proguardLogsProcessor) processLogRecordThrow(ctx context.Context, attributes pcommon.Map) error {
	var ok bool
	var classes, methods, lines pcommon.Slice

	if classes, ok = getSlice(p.cfg.ClassesAttributeKey, attributes); !ok {
		return fmt.Errorf("%w: %s", errMissingAttribute, p.cfg.ClassesAttributeKey)
	}

	if methods, ok = getSlice(p.cfg.MethodsAttributeKey, attributes); !ok {
		return fmt.Errorf("%w: %s", errMissingAttribute, p.cfg.MethodsAttributeKey)
	}

	if lines, ok = getSlice(p.cfg.LinesAttributeKey, attributes); !ok {
		return fmt.Errorf("%w: %s", errMissingAttribute, p.cfg.LinesAttributeKey)
	}

	// Ensure all slices are the same length
	if classes.Len() != methods.Len() || classes.Len() != lines.Len() {
		return fmt.Errorf("%w: (%s %d) (%s %d) (%s %d)", errMismatchedLength,
			p.cfg.ClassesAttributeKey, classes.Len(),
			p.cfg.MethodsAttributeKey, methods.Len(),
			p.cfg.LinesAttributeKey, lines.Len(),
		)
	}

	// Ensure all slices are the same length
	if classes.Len() != methods.Len() || classes.Len() != lines.Len() {
		return fmt.Errorf("%w: (%s %d) (%s %d) (%s %d)", errMismatchedLength,
			p.cfg.ClassesAttributeKey, classes.Len(),
			p.cfg.MethodsAttributeKey, methods.Len(),
			p.cfg.LinesAttributeKey, lines.Len(),
		)
	}

	if p.cfg.PreserveStackTrace {
		classes.CopyTo(attributes.PutEmptySlice(p.cfg.OriginalClassesAttributeKey))
		methods.CopyTo(attributes.PutEmptySlice(p.cfg.OriginalMethodsAttributeKey))
		lines.CopyTo(attributes.PutEmptySlice(p.cfg.OriginalLinesAttributeKey))

		if originalStackTrace, ok := attributes.Get(p.cfg.OutputStackTraceKey); ok {
			attributes.PutStr(p.cfg.OriginalStackTraceKey, originalStackTrace.Str())
		}
	}

	uuidValue, ok := attributes.Get(p.cfg.ProguardUUIDAttributeKey)
	if !ok {
		return fmt.Errorf("%w: %s", errMissingAttribute, p.cfg.ProguardUUIDAttributeKey)
	}

	uuid := uuidValue.Str()

	var stack []string
	var mappedClasses = attributes.PutEmptySlice(p.cfg.ClassesAttributeKey)
	var mappedMethods = attributes.PutEmptySlice(p.cfg.MethodsAttributeKey)
	var mappedLines = attributes.PutEmptySlice(p.cfg.LinesAttributeKey)

	var symbolicationFailed bool

	var exceptionType, _ = attributes.Get(p.cfg.ExceptionTypeKey)
	var exceptionMessage, _ = attributes.Get(p.cfg.ExceptionMessageKey)

	stack = append(stack, fmt.Sprintf("%s: %s", exceptionType.Str(), exceptionMessage.Str()))

	for i := 0; i < classes.Len(); i++ {
		line := lines.At(i).Int()

		if line < 0 || line > math.MaxUint32 {
			stack = append(stack, fmt.Sprintf("\tInvalid line number %d for %s.%s", line, classes.At(i).Str(), methods.At(i).Str()))
			symbolicationFailed = true
			continue
		}

		// maybe we should change this to take uint32?
		mappedClass, err := p.symbolicator.symbolicate(ctx, uuid, classes.At(i).Str(), methods.At(i).Str(), int(line))
		if err != nil {
			stack = append(stack, fmt.Sprintf("\tFailed to symbolicate %s.%s(%d): %v", classes.At(i).Str(), methods.At(i).Str(), line, err))
			symbolicationFailed = true
			continue
		}

		for _, mappedClass := range mappedClass {
			mappedClasses.AppendEmpty().SetStr(mappedClass.ClassName)
			mappedMethods.AppendEmpty().SetStr(mappedClass.MethodName)
			mappedLines.AppendEmpty().SetInt(mappedClass.LineNumber)

			stack = append(stack, fmt.Sprintf("\tat %s.%s(%s:%d)", mappedClass.ClassName, mappedClass.MethodName, mappedClass.SourceFile, mappedClass.LineNumber))
		}
	}

	attributes.PutStr(p.cfg.OutputStackTraceKey, strings.Join(stack, "\n"))

	if symbolicationFailed {
		return errPartialSymbolication
	} else {
		return nil
	}
}

func newProguardLogsProcessor(ctx context.Context, cfg *Config, store fileStore, set processor.Settings, symbolicator symbolicator) (*proguardLogsProcessor, error) {
	return &proguardLogsProcessor{
		cfg:          cfg,
		logger:       set.Logger,
		symbolicator: symbolicator,
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
