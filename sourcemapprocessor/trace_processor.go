package sourcemapprocessor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/honeycombio/opentelemetry-collector-symbolicator/sourcemapprocessor/internal/metadata"
	"go.opentelemetry.io/collector/pdata/pcommon"
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

	startTime := time.Now()
	for i := 0; i < td.ResourceSpans().Len(); i++ {
		rs := td.ResourceSpans().At(i)
		sp.processResourceSpans(ctx, rs)
	}

	sp.telemetryBuilder.ProcessorSymbolicationDuration.Record(ctx, time.Since(startTime).Seconds(), sp.attributes)
	return td, nil
}

// processResourceSpans takes resource spans and processes the attributes
// found on the spans.
func (sp *symbolicatorProcessor) processResourceSpans(ctx context.Context, rs ptrace.ResourceSpans) {
	for i := 0; i < rs.ScopeSpans().Len(); i++ {
		ss := rs.ScopeSpans().At(i)

		for j := 0; j < ss.Spans().Len(); j++ {
			span := ss.Spans().At(j)

			err := sp.processAttributes(ctx, span.Attributes(), rs.Resource().Attributes())

			if err != nil {
				sp.logger.Debug("Error processing span", zap.Error(err))
			}
		}
	}
}

// formatStackFrame takes a MappedStackFrame struct and returns a string representation of the stack frame
// TODO: Update to consider different browser formats
func formatStackFrame(sf *mappedStackFrame) string {
	return fmt.Sprintf("    at %s(%s:%d:%d)", sf.FunctionName, sf.URL, sf.Line, sf.Col)
}

// processAttributes takes the attributes of a span and returns an error if symbolication failed.
func (sp *symbolicatorProcessor) processAttributes(ctx context.Context, attributes pcommon.Map, resourceAttributes pcommon.Map) error {
	// Add processor type and version as attributes
	attributes.PutStr("honeycomb.processor_type", typeStr.String())
	attributes.PutStr("honeycomb.processor_version", processorVersion)

	err := sp.processThrow(ctx, attributes, resourceAttributes)

	if err != nil {
		attributes.PutBool(sp.cfg.SymbolicatorFailureAttributeKey, true)
		return err
	} else {
		attributes.PutBool(sp.cfg.SymbolicatorFailureAttributeKey, false)
		return nil
	}
}

// processThrow takes the attributes and determines if they contain
// required stacktrace information. If they do, it symbolicates the stack
// trace and adds it to the attributes.
func (sp *symbolicatorProcessor) processThrow(ctx context.Context, attributes pcommon.Map, resourceAttributes pcommon.Map) error {
	var ok bool
	var symbolicationError error
	var lines, columns, functions, urls pcommon.Slice

	if columns, ok = getSlice(sp.cfg.ColumnsAttributeKey, attributes); !ok {
		return fmt.Errorf("%w: %s", errMissingAttribute, sp.cfg.ColumnsAttributeKey)
	}
	if functions, ok = getSlice(sp.cfg.FunctionsAttributeKey, attributes); !ok {
		return fmt.Errorf("%w: %s", errMissingAttribute, sp.cfg.FunctionsAttributeKey)
	}
	if lines, ok = getSlice(sp.cfg.LinesAttributeKey, attributes); !ok {
		return fmt.Errorf("%w: %s", errMissingAttribute, sp.cfg.LinesAttributeKey)
	}
	if urls, ok = getSlice(sp.cfg.UrlsAttributeKey, attributes); !ok {
		return fmt.Errorf("%w: %s", errMissingAttribute, sp.cfg.UrlsAttributeKey)
	}

	// Ensure all slices are the same length
	if columns.Len() != functions.Len() || columns.Len() != lines.Len() || columns.Len() != urls.Len() {
		return fmt.Errorf("%w: (%s %d) (%s %d) (%s %d) (%s %d)", errMismatchedLength,
			sp.cfg.ColumnsAttributeKey, columns.Len(),
			sp.cfg.FunctionsAttributeKey, functions.Len(),
			sp.cfg.LinesAttributeKey, lines.Len(),
			sp.cfg.UrlsAttributeKey, urls.Len(),
		)
	}

	// Preserve original stack trace
	if sp.cfg.PreserveStackTrace {
		var origColumns = attributes.PutEmptySlice(sp.cfg.OriginalColumnsAttributeKey)
		columns.CopyTo(origColumns)

		var origFunctions = attributes.PutEmptySlice(sp.cfg.OriginalFunctionsAttributeKey)
		functions.CopyTo(origFunctions)

		var origLines = attributes.PutEmptySlice(sp.cfg.OriginalLinesAttributeKey)
		lines.CopyTo(origLines)

		var origUrls = attributes.PutEmptySlice(sp.cfg.OriginalUrlsAttributeKey)
		urls.CopyTo(origUrls)

		var origStackTraceStr, _ = attributes.Get(sp.cfg.OutputStackTraceKey)
		attributes.PutStr(sp.cfg.OriginalStackTraceKey, origStackTraceStr.Str())
	}

	buildUUID := ""
	if buildUUIDValue, ok := resourceAttributes.Get(sp.cfg.BuildUUIDAttributeKey); ok {
		buildUUID = buildUUIDValue.Str()
	}

	// Update with symbolicated stack trace
	var stack []string
	var mappedColumns = attributes.PutEmptySlice(sp.cfg.ColumnsAttributeKey)
	var mappedFunctions = attributes.PutEmptySlice(sp.cfg.FunctionsAttributeKey)
	var mappedLines = attributes.PutEmptySlice(sp.cfg.LinesAttributeKey)
	var mappedUrls = attributes.PutEmptySlice(sp.cfg.UrlsAttributeKey)

	var stackType, _ = attributes.Get(sp.cfg.StackTypeKey)
	var stackMessage, _ = attributes.Get(sp.cfg.StackMessageKey)

	stack = append(stack, fmt.Sprintf("%s: %s", stackType.Str(), stackMessage.Str()))

	// Cache fetch errors (404, timeout) to avoid redundant fetches.
	// Only FetchError types are cached - validation and parse errors are not cached
	// as they are frame-specific or indicate transient issues that might be resolved.
	// Note: Successful symbolications vary by line/column position, so can't be cached.
	fetchErrorCache := make(map[string]error)

	var hasSymbolicationFailed bool
	for i := 0; i < columns.Len(); i++ {
		url := urls.At(i).Str()
		line := lines.At(i).Int()
		column := columns.At(i).Int()
		function := functions.At(i).Str()

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
			hasSymbolicationFailed = true
			symbolicationError = err
			stack = append(stack, fmt.Sprintf("Failed to symbolicate: %v", err))
			mappedColumns.AppendEmpty().SetInt(-1)
			mappedFunctions.AppendEmpty().SetStr("")
			mappedLines.AppendEmpty().SetInt(-1)
			mappedUrls.AppendEmpty().SetStr("")

			sp.telemetryBuilder.ProcessorTotalFailedFrames.Add(ctx, 1, sp.attributes)
		} else {
			s := formatStackFrame(mappedStackFrame)
			stack = append(stack, s)
			mappedColumns.AppendEmpty().SetInt(mappedStackFrame.Col)
			mappedFunctions.AppendEmpty().SetStr(mappedStackFrame.FunctionName)
			mappedLines.AppendEmpty().SetInt(mappedStackFrame.Line)
			mappedUrls.AppendEmpty().SetStr(mappedStackFrame.URL)
		}
	}

	if symbolicationError != nil {
		attributes.PutStr(sp.cfg.SymbolicatorFailureMessageAttributeKey, symbolicationError.Error())
	}
	attributes.PutStr(sp.cfg.OutputStackTraceKey, strings.Join(stack, "\n"))

	if hasSymbolicationFailed {
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
