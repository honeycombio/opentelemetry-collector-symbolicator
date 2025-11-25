package sourcemapprocessor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/honeycombio/opentelemetry-collector-symbolicator/sourcemapprocessor/internal/metadata"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/ptrace"
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
	symbolicate(ctx context.Context, line, column int64, function, url string, uuid string) (*mappedStackFrame, error)
}

// sourcemapprocessor is a processor that finds and symbolicates stack
// traces that it finds in the attributes of spans.
type symbolicatorProcessor struct {
	logger *zap.Logger

	cfg *Config

	symbolicator symbolicator

	telemetryBuilder *metadata.TelemetryBuilder
	attributes       metric.MeasurementOption
}

// newSymbolicatorProcessor creates a new symbolicatorProcessor.
func newSymbolicatorProcessor(_ context.Context, cfg *Config, set processor.Settings, symbolicator symbolicator, tb *metadata.TelemetryBuilder, attributes attribute.Set) *symbolicatorProcessor {
	return &symbolicatorProcessor{
		cfg:              cfg,
		logger:           set.Logger,
		symbolicator:     symbolicator,
		telemetryBuilder: tb,
		attributes:       metric.WithAttributeSet(attributes),
	}
}

// processTraces processes the received traces. It is the function configured
// in the processorhelper.NewTraces call in factory.go
func (sp *symbolicatorProcessor) processTraces(ctx context.Context, td ptrace.Traces) (ptrace.Traces, error) {
	sp.logger.Debug("Processing traces")

	for i := 0; i < td.ResourceSpans().Len(); i++ {
		rs := td.ResourceSpans().At(i)
		sp.processResourceSpans(ctx, rs)
	}

	return td, nil
}

// processResourceSpans takes resource spans and processes the attributes
// found on the spans.
func (sp *symbolicatorProcessor) processResourceSpans(ctx context.Context, rs ptrace.ResourceSpans) {
	for i := 0; i < rs.ScopeSpans().Len(); i++ {
		ss := rs.ScopeSpans().At(i)

		for j := 0; j < ss.Spans().Len(); j++ {
			span := ss.Spans().At(j)
			sp.processAttributes(ctx, span.Attributes(), rs.Resource().Attributes())
		}
	}
}

// processLogs processes the received logs. It is the function configured
// in the processorhelper.NewLogs call in factory.go
func (sp *symbolicatorProcessor) processLogs(ctx context.Context, logs plog.Logs) (plog.Logs, error) {
	sp.logger.Debug("Processing logs")

	for i := 0; i < logs.ResourceLogs().Len(); i++ {
		rl := logs.ResourceLogs().At(i)
		sp.processResourceLogs(ctx, rl)
	}

	return logs, nil
}

// processResourceLogs takes resource logs and processes the attributes
// found on the log records.
func (sp *symbolicatorProcessor) processResourceLogs(ctx context.Context, rl plog.ResourceLogs) {
	for i := 0; i < rl.ScopeLogs().Len(); i++ {
		sl := rl.ScopeLogs().At(i)

		for j := 0; j < sl.LogRecords().Len(); j++ {
			logRecord := sl.LogRecords().At(j)
			sp.processAttributes(ctx, logRecord.Attributes(), rl.Resource().Attributes())
		}
	}
}

// formatStackFrame takes a MappedStackFrame struct and returns a string representation of the stack frame
// TODO: Update to consider different browser formats
func formatStackFrame(sf *mappedStackFrame) string {
	return fmt.Sprintf("    at %s(%s:%d:%d)", sf.FunctionName, sf.URL, sf.Line, sf.Col)
}

// processAttributes takes the attributes of a span and returns an error if symbolication failed.
func (sp *symbolicatorProcessor) processAttributes(ctx context.Context, attributes pcommon.Map, resourceAttributes pcommon.Map) {
	// Skip all processing if StackTraceAttributeKey is not present
	if _, ok := attributes.Get(sp.cfg.StackTraceAttributeKey); !ok {
		return
	}

	// Start timing symbolication only when we actually perform it
	// End timing deferred to after processing is done
	startTime := time.Now()
	defer func() {
		sp.telemetryBuilder.ProcessorSymbolicationDuration.Record(ctx, time.Since(startTime).Seconds(), sp.attributes)
	}()

	// Add processor type and version as attributes
	attributes.PutStr("honeycomb.processor_type", typeStr.String())
	attributes.PutStr("honeycomb.processor_version", processorVersion)

	err := sp.processThrow(ctx, attributes, resourceAttributes)

	if err != nil {
		attributes.PutBool(sp.cfg.SymbolicatorFailureAttributeKey, true)
		attributes.PutStr(sp.cfg.SymbolicatorErrorAttributeKey, err.Error())
	} else {
		attributes.PutBool(sp.cfg.SymbolicatorFailureAttributeKey, false)
	}
}

// processThrow takes the attributes and determines if they contain
// required stacktrace information. If they do, it symbolicates the stack
// trace and adds it to the attributes.
func (sp *symbolicatorProcessor) processThrow(ctx context.Context, attributes pcommon.Map, resourceAttributes pcommon.Map) error {
	var lines, columns, functions, urls pcommon.Slice
	var hasLines, hasColumns, hasFunctions, hasUrls bool

	var exceptionType, hasExceptionType = attributes.Get(sp.cfg.ExceptionTypeAttributeKey)
	var exceptionMessage, hasExceptionMessage = attributes.Get(sp.cfg.ExceptionMessageAttributeKey)

	// Attempt to get structured stack trace attributes first
	lines, hasLines = getSlice(sp.cfg.LinesAttributeKey, attributes)
	columns, hasColumns = getSlice(sp.cfg.ColumnsAttributeKey, attributes)
	functions, hasFunctions = getSlice(sp.cfg.FunctionsAttributeKey, attributes)
	urls, hasUrls = getSlice(sp.cfg.UrlsAttributeKey, attributes)
	rawStackTrace, hasRawStackTrace := attributes.Get(sp.cfg.StackTraceAttributeKey)

	// If any of the structured attributes are missing, attempt to parse the raw stack trace
	var parsedStackTrace *StackTrace
	if !hasLines || !hasColumns || !hasFunctions || !hasUrls {
		if !hasRawStackTrace {
			return fmt.Errorf("%w: missing structured stack trace attributes and %s attribute is missing",
				errMissingAttribute,
				sp.cfg.StackTraceAttributeKey,
			)
		}

		tk := NewTraceKit()
		parsedStackTrace = tk.ComputeStackTrace(exceptionType.Str(), exceptionMessage.Str(), rawStackTrace.Str(), 0)

		// Check if parsing failed
		if parsedStackTrace.Mode == "failed" {
			return fmt.Errorf("failed to parse raw stack trace from %s", sp.cfg.StackTraceAttributeKey)
		}

		attributes.PutStr(sp.cfg.ExceptionTypeAttributeKey, parsedStackTrace.Name)
		exceptionType, hasExceptionType = attributes.Get(sp.cfg.ExceptionTypeAttributeKey)

		attributes.PutStr(sp.cfg.ExceptionMessageAttributeKey, parsedStackTrace.Message)
		exceptionMessage, hasExceptionMessage = attributes.Get(sp.cfg.ExceptionMessageAttributeKey)

		attributes.PutStr(sp.cfg.SymbolicatorParsingMethodAttributeKey, "processor_parsed")
	} else {
		attributes.PutStr(sp.cfg.SymbolicatorParsingMethodAttributeKey, "structured_stacktrace_attributes")
	}

	buildUUID := ""
	if buildUUIDValue, ok := resourceAttributes.Get(sp.cfg.BuildUUIDAttributeKey); ok {
		buildUUID = buildUUIDValue.Str()
	}

	var stack []string
	var symbolicationFailed bool

	// Reconstruct the stack trace with symbolicated frames
	if hasExceptionType && hasExceptionMessage {
		stack = append(stack, fmt.Sprintf("%s: %s", exceptionType.Str(), exceptionMessage.Str()))
	}

	// Cache FetchErrors to avoid redundant fetches for missing resources.
	fetchErrorCache := make(map[string]error)

	// Set up iteration and output slices based on route
	var mappedColumns, mappedFunctions, mappedLines, mappedUrls pcommon.Slice
	var iterCount int

	// Set up iteration based on whether we have a parsed stack trace or structured attributes
	if parsedStackTrace != nil {
		iterCount = len(parsedStackTrace.Stack)

		if sp.cfg.PreserveStackTrace {
			attributes.PutStr(sp.cfg.OriginalStackTraceAttributeKey, rawStackTrace.Str())
		}
	} else {
		iterCount = columns.Len()
		mappedColumns = attributes.PutEmptySlice(sp.cfg.ColumnsAttributeKey)
		mappedFunctions = attributes.PutEmptySlice(sp.cfg.FunctionsAttributeKey)
		mappedLines = attributes.PutEmptySlice(sp.cfg.LinesAttributeKey)
		mappedUrls = attributes.PutEmptySlice(sp.cfg.UrlsAttributeKey)

		// Ensure all slices are the same length
		if columns.Len() != functions.Len() || columns.Len() != lines.Len() || columns.Len() != urls.Len() {
			return fmt.Errorf("%w: (%s %d) (%s %d) (%s %d) (%s %d)", errMismatchedLength,
				sp.cfg.ColumnsAttributeKey, columns.Len(),
				sp.cfg.FunctionsAttributeKey, functions.Len(),
				sp.cfg.LinesAttributeKey, lines.Len(),
				sp.cfg.UrlsAttributeKey, urls.Len(),
			)
		}

		if sp.cfg.PreserveStackTrace {
			columns.CopyTo(attributes.PutEmptySlice(sp.cfg.OriginalColumnsAttributeKey))
			functions.CopyTo(attributes.PutEmptySlice(sp.cfg.OriginalFunctionsAttributeKey))
			lines.CopyTo(attributes.PutEmptySlice(sp.cfg.OriginalLinesAttributeKey))
			urls.CopyTo(attributes.PutEmptySlice(sp.cfg.OriginalUrlsAttributeKey))
			attributes.PutStr(sp.cfg.OriginalStackTraceAttributeKey, rawStackTrace.Str())
		}
	}

	for i := 0; i < iterCount; i++ {
		var url, function string
		var line, column int64

		// Get frame data based on route
		if parsedStackTrace != nil {
			// Extract from parsed frame
			frame := parsedStackTrace.Stack[i]
			url = frame.URL
			function = frame.Func
			if frame.Line != nil {
				line = int64(*frame.Line)
			} else {
				line = -1
			}
			if frame.Column != nil {
				column = int64(*frame.Column)
			} else {
				column = -1
			}
		} else {
			// Extract from structured attributes
			url = urls.At(i).Str()
			line = lines.At(i).Int()
			column = columns.At(i).Int()
			function = functions.At(i).Str()
		}

		cacheKey := buildCacheKey(url, buildUUID)

		sp.telemetryBuilder.ProcessorTotalProcessedFrames.Add(ctx, 1, sp.attributes)

		var mappedStackFrame *mappedStackFrame
		var err error

		// Check if we have a cached fetch error for this URL
		if cachedError, exists := fetchErrorCache[cacheKey]; exists {
			err = cachedError
		} else {
			mappedStackFrame, err = sp.symbolicator.symbolicate(ctx, line, column, function, url, buildUUID)

			// Only cache FetchErrors (404, timeout, etc.) - not validation or parse errors
			if err != nil {
				var fetchErr *FetchError
				if errors.As(err, &fetchErr) {
					fetchErrorCache[cacheKey] = err
				}
			}
		}

		if err != nil {
			symbolicationFailed = true
			stack = append(stack, fmt.Sprintf("\tFailed to symbolicate %s at %s:%d:%d: %v", function, url, line, column, err))

			// Only populate output slices for structured route
			if parsedStackTrace == nil {
				mappedColumns.AppendEmpty().SetInt(-1)
				mappedFunctions.AppendEmpty().SetStr("")
				mappedLines.AppendEmpty().SetInt(-1)
				mappedUrls.AppendEmpty().SetStr("")
			}

			sp.telemetryBuilder.ProcessorTotalFailedFrames.Add(ctx, 1, sp.attributes)
		} else {
			s := formatStackFrame(mappedStackFrame)
			stack = append(stack, s)

			// Only populate output slices for structured route
			if parsedStackTrace == nil {
				mappedColumns.AppendEmpty().SetInt(mappedStackFrame.Col)
				mappedFunctions.AppendEmpty().SetStr(mappedStackFrame.FunctionName)
				mappedLines.AppendEmpty().SetInt(mappedStackFrame.Line)
				mappedUrls.AppendEmpty().SetStr(mappedStackFrame.URL)
			}
		}
	}

	attributes.PutStr(sp.cfg.StackTraceAttributeKey, strings.Join(stack, "\n"))

	if symbolicationFailed {
		return errPartialSymbolication
	} else {
		return nil
	}
}

// getSlice retrieves a slice from a map, returning an empty slice if the key is not found.
func getSlice(key string, m pcommon.Map) (pcommon.Slice, bool) {
	v, ok := m.Get(key)
	if !ok {
		return pcommon.NewSlice(), false
	}

	return v.Slice(), true
}
