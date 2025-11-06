package proguardprocessor

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/honeycombio/opentelemetry-collector-symbolicator/proguardprocessor/internal/metadata"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

var (
	errMissingAttribute     = errors.New("missing attribute")
	errMismatchedLength     = errors.New("mismatched stacktrace attribute lengths")
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

	telemetryBuilder *metadata.TelemetryBuilder
	attributes       metric.MeasurementOption
}

func (p *proguardLogsProcessor) ProcessLogs(ctx context.Context, logs plog.Logs) (plog.Logs, error) {
	p.logger.Debug("Processing logs")

	startTime := time.Now()
	for i := 0; i < logs.ResourceLogs().Len(); i++ {
		rl := logs.ResourceLogs().At(i)
		p.processResourceLogs(ctx, rl)
	}

	p.telemetryBuilder.ProcessorSymbolicationDuration.Record(ctx, time.Since(startTime).Seconds(), p.attributes)
	return logs, nil
}

func (p *proguardLogsProcessor) processResourceLogs(ctx context.Context, rl plog.ResourceLogs) {
	for j := 0; j < rl.ScopeLogs().Len(); j++ {
		sl := rl.ScopeLogs().At(j)
		p.processScopeLogs(ctx, sl)
	}
}

func (p *proguardLogsProcessor) processScopeLogs(ctx context.Context, sl plog.ScopeLogs) {
	for k := 0; k < sl.LogRecords().Len(); k++ {
		lr := sl.LogRecords().At(k)
		p.processLogRecord(ctx, lr)
	}
}

func (p *proguardLogsProcessor) processLogRecord(ctx context.Context, lr plog.LogRecord) {
	attributes := lr.Attributes()

	// Add processor type and version as attributes
	attributes.PutStr("honeycomb.processor_type", typeStr.String())
	attributes.PutStr("honeycomb.processor_version", processorVersion)

	err := p.processLogRecordThrow(ctx, attributes)

	if err != nil {
		attributes.PutBool(p.cfg.SymbolicatorFailureAttributeKey, true)
		attributes.PutStr(p.cfg.SymbolicatorErrorAttributeKey, err.Error())
	} else {
		attributes.PutBool(p.cfg.SymbolicatorFailureAttributeKey, false)
	}
}

func (p *proguardLogsProcessor) processLogRecordThrow(ctx context.Context, attributes pcommon.Map) error {
	var ok bool
	var classes, methods, lines, sourceFiles pcommon.Slice
	var classesOk, methodsOk, linesOk, sourceFilesOk bool

	var exceptionType, hasExceptionType = attributes.Get(p.cfg.ExceptionTypeAttributeKey)
	var exceptionMessage, hasExceptionMessage = attributes.Get(p.cfg.ExceptionMessageAttributeKey)

	// Attempt to get structured stack trace attributes first
	classes, classesOk = getSlice(p.cfg.ClassesAttributeKey, attributes)
	methods, methodsOk = getSlice(p.cfg.MethodsAttributeKey, attributes)
	lines, linesOk = getSlice(p.cfg.LinesAttributeKey, attributes)
	sourceFiles, sourceFilesOk = getSlice(p.cfg.SourceFilesAttributeKey, attributes)

	// If any of the structured attributes are missing, attempt to parse the raw stack trace
	if !classesOk || !methodsOk || !linesOk || !sourceFilesOk {
		rawStackTrace, hasRawStackTrace := attributes.Get(p.cfg.StackTraceAttributeKey)

		if !hasRawStackTrace {
			return fmt.Errorf("%w: missing structured stack trace attributes and %s attribute is missing",
				errMissingAttribute,
				p.cfg.StackTraceAttributeKey,
			)
		}

		parsedStackTrace, err := parseStackTrace(rawStackTrace.Str())
		if err != nil {
			return fmt.Errorf("failed to parse raw stack trace from %s: %w", p.cfg.StackTraceAttributeKey, err)
		}

		// Set parsed data into otel slice attributes
		classes = attributes.PutEmptySlice(p.cfg.ClassesAttributeKey)
		methods = attributes.PutEmptySlice(p.cfg.MethodsAttributeKey)
		lines = attributes.PutEmptySlice(p.cfg.LinesAttributeKey)
		sourceFiles = attributes.PutEmptySlice(p.cfg.SourceFilesAttributeKey)

		// sadly :( we need to loop again to populate the slices into otel structures
		for _, frame := range parsedStackTrace.frames {
			classes.AppendEmpty().SetStr(frame.class)
			methods.AppendEmpty().SetStr(frame.method)
			lines.AppendEmpty().SetInt(int64(frame.line))
			sourceFiles.AppendEmpty().SetStr(frame.sourceFile)
		}

		if !hasExceptionType {
			attributes.PutStr(p.cfg.ExceptionTypeAttributeKey, parsedStackTrace.exceptionType)
			exceptionType, _ = attributes.Get(p.cfg.ExceptionTypeAttributeKey)
			hasExceptionType = true
		}

		if !hasExceptionMessage {
			attributes.PutStr(p.cfg.ExceptionMessageAttributeKey, parsedStackTrace.exceptionMessage)
			exceptionMessage, _ = attributes.Get(p.cfg.ExceptionMessageAttributeKey)
			hasExceptionMessage = true
		}
	}

	// Ensure all slices are the same length
	if classes.Len() != methods.Len() || classes.Len() != lines.Len() || classes.Len() != sourceFiles.Len() {
		return fmt.Errorf("%w: (%s %d) (%s %d) (%s %d) (%s %d)", errMismatchedLength,
			p.cfg.ClassesAttributeKey, classes.Len(),
			p.cfg.MethodsAttributeKey, methods.Len(),
			p.cfg.LinesAttributeKey, lines.Len(),
			p.cfg.SourceFilesAttributeKey, sourceFiles.Len(),
		)
	}

	if p.cfg.PreserveStackTrace {
		classes.CopyTo(attributes.PutEmptySlice(p.cfg.OriginalClassesAttributeKey))
		methods.CopyTo(attributes.PutEmptySlice(p.cfg.OriginalMethodsAttributeKey))
		lines.CopyTo(attributes.PutEmptySlice(p.cfg.OriginalLinesAttributeKey))

		if originalStackTrace, ok := attributes.Get(p.cfg.StackTraceAttributeKey); ok {
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

	// Reconstruct the stack trace with symbolicated frames
	if hasExceptionType && hasExceptionMessage {
		stack = append(stack, fmt.Sprintf("%s: %s", exceptionType.Str(), exceptionMessage.Str()))
	}
	for i := 0; i < classes.Len(); i++ {
		line := lines.At(i).Int()

		// Line numbers set to -2 and -1 are special values indicating a native method and unknown source respectively, per the Android docs.
		if line < -2 || line > math.MaxUint32 {
			stack = append(stack, fmt.Sprintf("\tInvalid line number %d for %s.%s", line, classes.At(i).Str(), methods.At(i).Str()))
			symbolicationFailed = true
			continue
		}

		// maybe we should change this to take uint32?
		mappedClass, err := p.symbolicator.symbolicate(ctx, uuid, classes.At(i).Str(), methods.At(i).Str(), int(line))
		p.telemetryBuilder.ProcessorTotalProcessedFrames.Add(ctx, 1, p.attributes)

		if err != nil {
			stack = append(stack, fmt.Sprintf("\tFailed to symbolicate %s.%s(%d): %v", classes.At(i).Str(), methods.At(i).Str(), line, err))
			symbolicationFailed = true
			p.telemetryBuilder.ProcessorTotalFailedFrames.Add(ctx, 1, p.attributes)
			continue
		}
		// Not a symbolication failure but no mapping found or needed; use original stacktrace data
		if len(mappedClass) == 0 {
			class := classes.At(i).Str()
			method := methods.At(i).Str()
			sourceFile := sourceFiles.At(i).Str()

			// since we are using original stacktrace data, preserve original values in the output slices
			mappedClasses.AppendEmpty().SetStr(class)
			mappedMethods.AppendEmpty().SetStr(method)
			mappedLines.AppendEmpty().SetInt(line)

			if line == -2 {
				// Native method, source file and line number are not applicable
				stack = append(stack, fmt.Sprintf("\tat %s.%s(Native Method)", class, method))
			} else if line == -1 {
				// Unknown source file and line number
				stack = append(stack, fmt.Sprintf("\tat %s.%s(Unknown Source)", class, method))
			} else {
				stack = append(stack, fmt.Sprintf("\tat %s.%s(%s:%d)", class, method, sourceFile, line))
			}
			continue
		}

		for _, mappedClass := range mappedClass {
			mappedClasses.AppendEmpty().SetStr(mappedClass.ClassName)
			mappedMethods.AppendEmpty().SetStr(mappedClass.MethodName)
			mappedLines.AppendEmpty().SetInt(mappedClass.LineNumber)

			stack = append(stack, fmt.Sprintf("\tat %s.%s(%s:%d)", mappedClass.ClassName, mappedClass.MethodName, mappedClass.SourceFile, mappedClass.LineNumber))
		}
	}

	attributes.PutStr(p.cfg.StackTraceAttributeKey, strings.Join(stack, "\n"))

	if symbolicationFailed {
		return errPartialSymbolication
	} else {
		return nil
	}
}

func newProguardLogsProcessor(ctx context.Context, cfg *Config, store fileStore, set processor.Settings, symbolicator symbolicator, tb *metadata.TelemetryBuilder, attributes attribute.Set) (*proguardLogsProcessor, error) {
	return &proguardLogsProcessor{
		cfg:              cfg,
		logger:           set.Logger,
		symbolicator:     symbolicator,
		telemetryBuilder: tb,
		attributes:       metric.WithAttributeSet(attributes),
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
