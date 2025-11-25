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

	for i := 0; i < logs.ResourceLogs().Len(); i++ {
		rl := logs.ResourceLogs().At(i)
		p.processResourceLogs(ctx, rl)
	}

	return logs, nil
}

func (p *proguardLogsProcessor) processResourceLogs(ctx context.Context, rl plog.ResourceLogs) {
	resourceAttrs := rl.Resource().Attributes()
	for j := 0; j < rl.ScopeLogs().Len(); j++ {
		sl := rl.ScopeLogs().At(j)
		p.processScopeLogs(ctx, sl, resourceAttrs)
	}
}

func (p *proguardLogsProcessor) processScopeLogs(ctx context.Context, sl plog.ScopeLogs, resourceAttrs pcommon.Map) {
	for k := 0; k < sl.LogRecords().Len(); k++ {
		lr := sl.LogRecords().At(k)
		p.processLogRecord(ctx, lr, resourceAttrs)
	}
}

func (p *proguardLogsProcessor) processLogRecord(ctx context.Context, lr plog.LogRecord, resourceAttrs pcommon.Map) {
	attributes := lr.Attributes()

	// Skip all processing if StackTraceAttributeKey is not present
	if _, ok := attributes.Get(p.cfg.StackTraceAttributeKey); !ok {
		return
	}

	// Start timing symbolication only when we actually perform it
	// End timing deferred to after processing is done
	startTime := time.Now()
	defer func() {
		p.telemetryBuilder.ProcessorSymbolicationDuration.Record(ctx, time.Since(startTime).Seconds(), p.attributes)
	}()

	// Add processor type and version as attributes
	attributes.PutStr("honeycomb.processor_type", typeStr.String())
	attributes.PutStr("honeycomb.processor_version", processorVersion)

	err := p.processLogRecordThrow(ctx, attributes, resourceAttrs)

	if err != nil {
		attributes.PutBool(p.cfg.SymbolicatorFailureAttributeKey, true)
		attributes.PutStr(p.cfg.SymbolicatorErrorAttributeKey, err.Error())
	} else {
		attributes.PutBool(p.cfg.SymbolicatorFailureAttributeKey, false)
	}
}

func (p *proguardLogsProcessor) processLogRecordThrow(ctx context.Context, attributes pcommon.Map, resourceAttrs pcommon.Map) error {
	var ok bool

	// Support retrieving the Proguard UUID from either resource or log attributes, for now.
	uuidValue, ok := attributes.Get(p.cfg.ProguardUUIDAttributeKey)
	if !ok {
		uuidValue, ok = resourceAttrs.Get(p.cfg.ProguardUUIDAttributeKey)
		if !ok {
			return fmt.Errorf("%w: %s", errMissingAttribute, p.cfg.ProguardUUIDAttributeKey)
		}
	}

	var classes, methods, lines, sourceFiles pcommon.Slice
	var hasClasses, hasMethods, hasLines, hasSourceFiles bool

	var exceptionType, hasExceptionType = attributes.Get(p.cfg.ExceptionTypeAttributeKey)
	var exceptionMessage, hasExceptionMessage = attributes.Get(p.cfg.ExceptionMessageAttributeKey)

	// Attempt to get structured stack trace attributes first
	classes, hasClasses = getSlice(p.cfg.ClassesAttributeKey, attributes)
	methods, hasMethods = getSlice(p.cfg.MethodsAttributeKey, attributes)
	lines, hasLines = getSlice(p.cfg.LinesAttributeKey, attributes)
	sourceFiles, hasSourceFiles = getSlice(p.cfg.SourceFilesAttributeKey, attributes)
	rawStackTrace, hasRawStackTrace := attributes.Get(p.cfg.StackTraceAttributeKey)

	// If any of the structured attributes are missing, attempt to parse the raw stack trace
	var parsedStackTrace *stackTrace
	var err error
	if !hasClasses || !hasMethods || !hasLines || !hasSourceFiles {
		if !hasRawStackTrace {
			return fmt.Errorf("%w: missing structured stack trace attributes and %s attribute is missing",
				errMissingAttribute,
				p.cfg.StackTraceAttributeKey,
			)
		}

		parsedStackTrace, err = parseStackTrace(rawStackTrace.Str())
		if err != nil {
			return fmt.Errorf("failed to parse raw stack trace from %s: %w", p.cfg.StackTraceAttributeKey, err)
		}

		attributes.PutStr(p.cfg.ExceptionTypeAttributeKey, parsedStackTrace.exceptionType)
		exceptionType, hasExceptionType = attributes.Get(p.cfg.ExceptionTypeAttributeKey)

		attributes.PutStr(p.cfg.ExceptionMessageAttributeKey, parsedStackTrace.exceptionMessage)
		exceptionMessage, hasExceptionMessage = attributes.Get(p.cfg.ExceptionMessageAttributeKey)

		attributes.PutStr(p.cfg.SymbolicatorParsingMethodAttributeKey, "processor_parsed")
	} else {
		attributes.PutStr(p.cfg.SymbolicatorParsingMethodAttributeKey, "structured_stacktrace_attributes")
	}

	uuid := uuidValue.Str()

	var stack []string
	var symbolicationFailed bool

	// Reconstruct the stack trace with symbolicated frames
	if hasExceptionType && hasExceptionMessage {
		stack = append(stack, fmt.Sprintf("%s: %s", exceptionType.Str(), exceptionMessage.Str()))
	}

	// Cache FetchErrors to avoid redundant fetches for missing resources.
	fetchErrorCache := make(map[string]error)

	// Set up iteration and output slices based on route
	var mappedClasses, mappedMethods, mappedLines pcommon.Slice
	var iterCount int

	// Set up iteration based on whether we have a parsed stack trace or structured attributes
	if parsedStackTrace != nil {
		iterCount = len(parsedStackTrace.elements)

		if p.cfg.PreserveStackTrace {
			attributes.PutStr(p.cfg.OriginalStackTraceAttributeKey, rawStackTrace.Str())
		}
	} else {
		iterCount = classes.Len()
		mappedClasses = attributes.PutEmptySlice(p.cfg.ClassesAttributeKey)
		mappedMethods = attributes.PutEmptySlice(p.cfg.MethodsAttributeKey)
		mappedLines = attributes.PutEmptySlice(p.cfg.LinesAttributeKey)

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
			sourceFiles.CopyTo(attributes.PutEmptySlice(p.cfg.OriginalSourceFilesAttributeKey))
			attributes.PutStr(p.cfg.OriginalStackTraceAttributeKey, rawStackTrace.Str())
		}
	}

	for i := 0; i < iterCount; i++ {
		var class, method, sourceFile string
		var line int64

		// Get frame data based on route
		if parsedStackTrace != nil {
			element := parsedStackTrace.elements[i]
			// Preserve raw lines that couldn't be parsed as frames
			if element.line != "" {
				stack = append(stack, element.line)
				continue
			}
			// Extract from parsed frame
			class = element.frame.class
			method = element.frame.method
			line = int64(element.frame.line)
			sourceFile = element.frame.sourceFile
		} else {
			// Extract from structured attributes
			class = classes.At(i).Str()
			method = methods.At(i).Str()
			line = lines.At(i).Int()
			sourceFile = sourceFiles.At(i).Str()
		}

		// Line numbers set to -2 and -1 are special values indicating a native method and unknown source respectively, per the Android docs.
		if line < -2 || line > math.MaxUint32 {
			stack = append(stack, fmt.Sprintf("\tInvalid line number %d for %s.%s", line, class, method))
			symbolicationFailed = true
			continue
		}

		p.telemetryBuilder.ProcessorTotalProcessedFrames.Add(ctx, 1, p.attributes)

		var mappedFrames []*mappedStackFrame
		var err error

		// Check if we have a cached fetch error for this UUID
		if cachedError, exists := fetchErrorCache[uuid]; exists {
			err = cachedError
		} else {
			mappedFrames, err = p.symbolicator.symbolicate(ctx, uuid, class, method, int(line))

			// Only cache FetchErrors (404, timeout, etc.) - not parse or validation errors
			if err != nil {
				var fetchErr *FetchError
				if errors.As(err, &fetchErr) {
					fetchErrorCache[uuid] = err
				}
			}
		}

		if err != nil {
			stack = append(stack, fmt.Sprintf("\tFailed to symbolicate %s.%s(%d): %v", class, method, line, err))
			symbolicationFailed = true
			p.telemetryBuilder.ProcessorTotalFailedFrames.Add(ctx, 1, p.attributes)
			continue
		}

		// Not a symbolication failure but no mapping found or needed; use original stacktrace data
		if len(mappedFrames) == 0 {
			// Only populate output slices for structured route
			if parsedStackTrace == nil {
				mappedClasses.AppendEmpty().SetStr(class)
				mappedMethods.AppendEmpty().SetStr(method)
				mappedLines.AppendEmpty().SetInt(line)
			}

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

		for _, mappedFrame := range mappedFrames {
			// Only populate output slices for structured route
			if parsedStackTrace == nil {
				mappedClasses.AppendEmpty().SetStr(mappedFrame.ClassName)
				mappedMethods.AppendEmpty().SetStr(mappedFrame.MethodName)
				mappedLines.AppendEmpty().SetInt(mappedFrame.LineNumber)
			}

			stack = append(stack, fmt.Sprintf("\tat %s.%s(%s:%d)", mappedFrame.ClassName, mappedFrame.MethodName, mappedFrame.SourceFile, mappedFrame.LineNumber))
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
