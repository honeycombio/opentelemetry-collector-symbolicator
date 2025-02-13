package symbolicatorprocessor

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

var (
	errMissingAttribute = errors.New("missing attribute")
	errMismatchedLength = errors.New("mismatched stacktrace attribute lengths")
)

// symbolicator interface is used to symbolicate stack traces.
type symbolicator interface {
	symbolicate(ctx context.Context, line, column int64, function, url string) (*mappedStackFrame, error)
}

// symbolicatorProcessor is a processor that finds and symbolicates stack
// traces that it finds in the attributes of spans.
type symbolicatorProcessor struct {
	logger *zap.Logger

	cfg *Config

	symbolicator symbolicator
}

// newSymbolicatorProcessor creates a new symbolicatorProcessor.
func newSymbolicatorProcessor(_ context.Context, cfg *Config, set processor.Settings, symbolicator symbolicator) *symbolicatorProcessor {
	return &symbolicatorProcessor{
		cfg:          cfg,
		logger:       set.Logger,
		symbolicator: symbolicator,
	}
}

// processTraces processes the received traces. It is the function configured
// in the processorhelper.NewTraces call in factory.go
func (sp *symbolicatorProcessor) processTraces(ctx context.Context, td ptrace.Traces) (ptrace.Traces, error) {
	sp.logger.Info("Processing traces")

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

			err := sp.processAttributes(ctx, span.Attributes())

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

// processAttributes takes the attributes and determines if they contain
// required stacktrace information. If they do, it symbolicates the stack
// trace and adds it to the attributes.
func (sp *symbolicatorProcessor) processAttributes(ctx context.Context, attributes pcommon.Map) error {
	var ok bool
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

	// Update with symbolicated stack trace
	var stack []string
	var mappedColumns = attributes.PutEmptySlice(sp.cfg.ColumnsAttributeKey)
	var mappedFunctions = attributes.PutEmptySlice(sp.cfg.FunctionsAttributeKey)
	var mappedLines = attributes.PutEmptySlice(sp.cfg.LinesAttributeKey)
	var mappedUrls = attributes.PutEmptySlice(sp.cfg.UrlsAttributeKey)

	var stackType, _ = attributes.Get(sp.cfg.StackTypeKey)
	var stackMessage, _ = attributes.Get(sp.cfg.StackMessageKey)

	stack = append(stack, fmt.Sprintf("%s: %s", stackType.Str(), stackMessage.Str()))

	for i := 0; i < columns.Len(); i++ {
		mappedStackFrame, err := sp.symbolicator.symbolicate(ctx, lines.At(i).Int(), columns.At(i).Int(), functions.At(i).Str(), urls.At(i).Str())

		if err != nil {
			stack = append(stack, fmt.Sprintf("Failed to symbolicate: %v", err))
			mappedColumns.AppendEmpty().SetInt(-1)
			mappedFunctions.AppendEmpty().SetStr("")
			mappedLines.AppendEmpty().SetInt(-1)
			mappedUrls.AppendEmpty().SetStr("")
		} else {
			s := formatStackFrame(mappedStackFrame)
			stack = append(stack, s)
			mappedColumns.AppendEmpty().SetInt(mappedStackFrame.Col)
			mappedFunctions.AppendEmpty().SetStr(mappedStackFrame.FunctionName)
			mappedLines.AppendEmpty().SetInt(mappedStackFrame.Line)
			mappedUrls.AppendEmpty().SetStr(mappedStackFrame.URL)
		}
	}

	attributes.PutStr(sp.cfg.OutputStackTraceKey, strings.Join(stack, "\n"))

	return nil
}

// getSlice retrieves a slice from a map, returning an empty slice if the key is not found.
func getSlice(key string, m pcommon.Map) (pcommon.Slice, bool) {
	v, ok := m.Get(key)
	if !ok {
		return pcommon.NewSlice(), false
	}

	return v.Slice(), true
}
