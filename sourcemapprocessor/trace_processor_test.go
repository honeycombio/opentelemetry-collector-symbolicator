package sourcemapprocessor

import (
	"context"
	"fmt"
	"math"
	"testing"

	"github.com/honeycombio/opentelemetry-collector-symbolicator/sourcemapprocessor/internal/metadata"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/otel/attribute"
	"go.uber.org/zap/zaptest"
)

type symbolicatedLine struct {
	Line     int64
	Column   int64
	Function string
	URL      string
}

type testSymbolicator struct {
	SymbolicatedLines []symbolicatedLine
}

func (ts *testSymbolicator) clear() {
	ts.SymbolicatedLines = nil
}

func (ts *testSymbolicator) symbolicate(ctx context.Context, line, column int64, function, url string) (*mappedStackFrame, error) {
	ts.SymbolicatedLines = append(ts.SymbolicatedLines, symbolicatedLine{
		Line:     line,
		Column:   column,
		Function: function,
		URL:      url,
	})

	// Special case for symbolication errors
	if column < 0 || column > math.MaxUint32 {
		return &mappedStackFrame{}, fmt.Errorf("column must be uint32: %d", column)
	}

	return &mappedStackFrame{FunctionName: function, Col: column, Line: line, URL: url}, nil
}

func TestProcess(t *testing.T) {
	ctx := context.Background()
	cfg := createDefaultConfig().(*Config)
	s := &testSymbolicator{}

	testTel := componenttest.NewTelemetry()
	tb, err := metadata.NewTelemetryBuilder(testTel.NewTelemetrySettings())
	assert.NoError(t, err)
	defer tb.Shutdown()

	attributes := attribute.NewSet(
		attribute.String("processor_type", "symbolicator"),
	)

	processor := newSymbolicatorProcessor(ctx, cfg, processor.Settings{
		TelemetrySettings: component.TelemetrySettings{
			Logger: zaptest.NewLogger(t),
		},
	}, s, tb, attributes)

	tts := []struct {
		Name                    string
		ApplyAttributes         func(attrs ptrace.Span)
		AssertSymbolicatorCalls func(s *testSymbolicator)
		AssertOutput            func(td ptrace.Traces)
	}{
		{
			Name: "symbolicated stacktrace attribute provided",
			ApplyAttributes: func(span ptrace.Span) {
				span.Attributes().PutEmpty(cfg.ColumnsAttributeKey).SetEmptySlice().AppendEmpty().SetInt(42)
				span.Attributes().PutEmpty(cfg.LinesAttributeKey).SetEmptySlice().AppendEmpty().SetInt(42)
				span.Attributes().PutEmpty(cfg.FunctionsAttributeKey).SetEmptySlice().AppendEmpty().SetStr("function")
				span.Attributes().PutEmpty(cfg.UrlsAttributeKey).SetEmptySlice().AppendEmpty().SetStr("url")
				span.Attributes().PutEmpty(cfg.StackTypeKey).SetStr("Error")
				span.Attributes().PutEmpty(cfg.StackMessageKey).SetStr("Test error!")
			},
			AssertSymbolicatorCalls: func(s *testSymbolicator) {
				assert.ElementsMatch(t, s.SymbolicatedLines, []symbolicatedLine{
					{Line: 42, Column: 42, Function: "function", URL: "url"},
				})
			},
			AssertOutput: func(td ptrace.Traces) {
				rs := td.ResourceSpans().At(0)
				ils := rs.ScopeSpans().At(0)
				span := ils.Spans().At(0)

				// Verify processor type and version attributes are included
				processorTypeAttr, ok := span.Attributes().Get("honeycomb.processor_type")
				assert.True(t, ok)
				assert.Equal(t, typeStr.String(), processorTypeAttr.Str())

				processorVersionAttr, ok := span.Attributes().Get("honeycomb.processor_version")
				assert.True(t, ok)
				assert.Equal(t, processorVersion, processorVersionAttr.Str())

				attr, ok := span.Attributes().Get(cfg.OutputStackTraceKey)
				assert.True(t, ok)
				assert.Equal(t, "Error: Test error!\n    at function(url:42:42)", attr.Str())

				attr, ok = span.Attributes().Get(cfg.ColumnsAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, "[42]", attr.AsString())

				attr, ok = span.Attributes().Get(cfg.LinesAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, "[42]", attr.AsString())

				attr, ok = span.Attributes().Get(cfg.FunctionsAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, "[\"function\"]", attr.AsString())

				attr, ok = span.Attributes().Get(cfg.UrlsAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, "[\"url\"]", attr.AsString())

				attr, ok = span.Attributes().Get(cfg.SymbolicatorFailureAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, false, attr.Bool())

				_, ok = span.Attributes().Get(cfg.SymbolicatorFailureMessageAttributeKey)
				assert.False(t, ok)
			},
		},
		{
			Name: "original stacktrace attributes preserved when preserveStackTrace is true (default)",
			ApplyAttributes: func(span ptrace.Span) {
				span.Attributes().PutEmpty(cfg.ColumnsAttributeKey).SetEmptySlice().FromRaw([]any{1, 2, 3})
				span.Attributes().PutEmpty(cfg.LinesAttributeKey).SetEmptySlice().FromRaw([]any{4, 5, 6})
				span.Attributes().PutEmpty(cfg.FunctionsAttributeKey).SetEmptySlice().FromRaw([]any{"func1", "func2", "func3"})
				span.Attributes().PutEmpty(cfg.UrlsAttributeKey).SetEmptySlice().FromRaw([]any{"url1", "url2", "url3"})
				span.Attributes().PutEmpty(cfg.OutputStackTraceKey).SetStr("Error: test error\n    at func1 (url1:4:1)\n    at func2 (url2:5:2)\n    at func3 (url3:6:3)")
			},
			AssertSymbolicatorCalls: func(s *testSymbolicator) {
				assert.ElementsMatch(t, s.SymbolicatedLines, []symbolicatedLine{
					{Line: 4, Column: 1, Function: "func1", URL: "url1"},
					{Line: 5, Column: 2, Function: "func2", URL: "url2"},
					{Line: 6, Column: 3, Function: "func3", URL: "url3"},
				})
			},
			AssertOutput: func(td ptrace.Traces) {
				rs := td.ResourceSpans().At(0)
				ils := rs.ScopeSpans().At(0)
				span := ils.Spans().At(0)
				attr, ok := span.Attributes().Get(cfg.OriginalColumnsAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, "[1,2,3]", attr.AsString())

				attr, ok = span.Attributes().Get(cfg.OriginalLinesAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, "[4,5,6]", attr.AsString())

				attr, ok = span.Attributes().Get(cfg.OriginalFunctionsAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, "[\"func1\",\"func2\",\"func3\"]", attr.AsString())

				attr, ok = span.Attributes().Get(cfg.OriginalUrlsAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, "[\"url1\",\"url2\",\"url3\"]", attr.AsString())

				attr, ok = span.Attributes().Get(cfg.OriginalStackTraceKey)
				assert.True(t, ok)
				assert.Equal(t, "Error: test error\n    at func1 (url1:4:1)\n    at func2 (url2:5:2)\n    at func3 (url3:6:3)", attr.AsString())
			},
		},
		{
			Name: "original stacktrace attributes not preserved when preserveStackTrace is false",
			ApplyAttributes: func(span ptrace.Span) {
				processor.cfg.PreserveStackTrace = false
				span.Attributes().PutEmpty(cfg.ColumnsAttributeKey).SetEmptySlice().FromRaw([]any{1, 2, 3})
				span.Attributes().PutEmpty(cfg.LinesAttributeKey).SetEmptySlice().FromRaw([]any{4, 5, 6})
				span.Attributes().PutEmpty(cfg.FunctionsAttributeKey).SetEmptySlice().FromRaw([]any{"func1", "func2", "func3"})
				span.Attributes().PutEmpty(cfg.UrlsAttributeKey).SetEmptySlice().FromRaw([]any{"url1", "url2", "url3"})
				span.Attributes().PutEmpty(cfg.OutputStackTraceKey).SetStr("Error: test error\n    at func1 (url1:4:1)\n    at func2 (url2:5:2)\n    at func3 (url3:6:3)")
			},
			AssertSymbolicatorCalls: func(s *testSymbolicator) {
				assert.ElementsMatch(t, s.SymbolicatedLines, []symbolicatedLine{
					{Line: 4, Column: 1, Function: "func1", URL: "url1"},
					{Line: 5, Column: 2, Function: "func2", URL: "url2"},
					{Line: 6, Column: 3, Function: "func3", URL: "url3"},
				})
			},
			AssertOutput: func(td ptrace.Traces) {
				rs := td.ResourceSpans().At(0)
				ils := rs.ScopeSpans().At(0)
				span := ils.Spans().At(0)
				_, ok := span.Attributes().Get(cfg.OriginalColumnsAttributeKey)
				assert.False(t, ok)

				_, ok = span.Attributes().Get(cfg.OriginalLinesAttributeKey)
				assert.False(t, ok)

				_, ok = span.Attributes().Get(cfg.OriginalFunctionsAttributeKey)
				assert.False(t, ok)

				_, ok = span.Attributes().Get(cfg.OriginalUrlsAttributeKey)
				assert.False(t, ok)

				_, ok = span.Attributes().Get(cfg.OriginalStackTraceKey)
				assert.False(t, ok)
			},
		},
		{
			Name: "missing columns attribute",
			ApplyAttributes: func(span ptrace.Span) {
				span.Attributes().PutEmpty(cfg.LinesAttributeKey).SetEmptySlice().AppendEmpty().SetInt(42)
				span.Attributes().PutEmpty(cfg.FunctionsAttributeKey).SetEmptySlice().AppendEmpty().SetStr("function")
				span.Attributes().PutEmpty(cfg.UrlsAttributeKey).SetEmptySlice().AppendEmpty().SetStr("url")
			},
			AssertSymbolicatorCalls: func(s *testSymbolicator) {
				assert.Empty(t, s.SymbolicatedLines)
			},
			AssertOutput: func(td ptrace.Traces) {},
		},
		{
			Name: "missing lines attribute",
			ApplyAttributes: func(span ptrace.Span) {
				span.Attributes().PutEmpty(cfg.ColumnsAttributeKey).SetEmptySlice().AppendEmpty().SetInt(42)
				span.Attributes().PutEmpty(cfg.FunctionsAttributeKey).SetEmptySlice().AppendEmpty().SetStr("function")
				span.Attributes().PutEmpty(cfg.UrlsAttributeKey).SetEmptySlice().AppendEmpty().SetStr("url")
			},
			AssertSymbolicatorCalls: func(s *testSymbolicator) {
				assert.Empty(t, s.SymbolicatedLines)
			},
			AssertOutput: func(td ptrace.Traces) {},
		},
		{
			Name: "missing functions attribute",
			ApplyAttributes: func(span ptrace.Span) {
				span.Attributes().PutEmpty(cfg.ColumnsAttributeKey).SetEmptySlice().AppendEmpty().SetInt(42)
				span.Attributes().PutEmpty(cfg.LinesAttributeKey).SetEmptySlice().AppendEmpty().SetInt(42)
				span.Attributes().PutEmpty(cfg.UrlsAttributeKey).SetEmptySlice().AppendEmpty().SetStr("url")
			},
			AssertSymbolicatorCalls: func(s *testSymbolicator) {
				assert.Empty(t, s.SymbolicatedLines)
			},
			AssertOutput: func(td ptrace.Traces) {},
		},
		{
			Name: "missing urls attribute",
			ApplyAttributes: func(span ptrace.Span) {
				span.Attributes().PutEmpty(cfg.ColumnsAttributeKey).SetEmptySlice().AppendEmpty().SetInt(42)
				span.Attributes().PutEmpty(cfg.LinesAttributeKey).SetEmptySlice().AppendEmpty().SetInt(42)
				span.Attributes().PutEmpty(cfg.FunctionsAttributeKey).SetEmptySlice().AppendEmpty().SetStr("function")
			},
			AssertSymbolicatorCalls: func(s *testSymbolicator) {
				assert.Empty(t, s.SymbolicatedLines)
			},
			AssertOutput: func(td ptrace.Traces) {},
		},
		{
			Name: "mismatched lengths",
			ApplyAttributes: func(span ptrace.Span) {
				slice := span.Attributes().PutEmpty(cfg.ColumnsAttributeKey).SetEmptySlice()
				slice.AppendEmpty().SetInt(42)
				slice.AppendEmpty().SetInt(42)
				span.Attributes().PutEmpty(cfg.LinesAttributeKey).SetEmptySlice().AppendEmpty().SetInt(42)
				span.Attributes().PutEmpty(cfg.FunctionsAttributeKey).SetEmptySlice().AppendEmpty().SetStr("function")
				span.Attributes().PutEmpty(cfg.UrlsAttributeKey).SetEmptySlice().AppendEmpty().SetStr("url")
			},
			AssertSymbolicatorCalls: func(s *testSymbolicator) {
				assert.Empty(t, s.SymbolicatedLines)
			},
			AssertOutput: func(td ptrace.Traces) {},
		},
		{
			Name: "symbolication failed attribute set to true on symbolication error",
			ApplyAttributes: func(span ptrace.Span) {
				span.Attributes().PutEmpty(cfg.ColumnsAttributeKey).SetEmptySlice().FromRaw([]any{1, int64(math.MaxUint32) + 1, 3})
				span.Attributes().PutEmpty(cfg.LinesAttributeKey).SetEmptySlice().FromRaw([]any{4, 5, 6})
				span.Attributes().PutEmpty(cfg.FunctionsAttributeKey).SetEmptySlice().FromRaw([]any{"func1", "func2", "func3"})
				span.Attributes().PutEmpty(cfg.UrlsAttributeKey).SetEmptySlice().FromRaw([]any{"url1", "url2", "url3"})
				span.Attributes().PutEmpty(cfg.OutputStackTraceKey).SetStr("Error: test error\n    at func1 (url1:4:1)\n    at func2 (url2:5:5000000000)\n    at func3 (url3:6:3)")
			},
			AssertSymbolicatorCalls: func(s *testSymbolicator) {
				assert.ElementsMatch(t, s.SymbolicatedLines, []symbolicatedLine{
					{Line: 4, Column: 1, Function: "func1", URL: "url1"},
					{Line: 5, Column: int64(math.MaxUint32) + 1, Function: "func2", URL: "url2"},
					{Line: 6, Column: 3, Function: "func3", URL: "url3"},
				})
			},
			AssertOutput: func(td ptrace.Traces) {
				rs := td.ResourceSpans().At(0)
				ils := rs.ScopeSpans().At(0)
				span := ils.Spans().At(0)

				// Verify processor type and version attributes are included even on failure
				processorTypeAttr, ok := span.Attributes().Get("honeycomb.processor_type")
				assert.True(t, ok)
				assert.Equal(t, typeStr.String(), processorTypeAttr.Str())

				processorVersionAttr, ok := span.Attributes().Get("honeycomb.processor_version")
				assert.True(t, ok)
				assert.Equal(t, processorVersion, processorVersionAttr.Str())

				attr, ok := span.Attributes().Get(cfg.SymbolicatorFailureAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, true, attr.Bool())
				attr, ok = span.Attributes().Get(cfg.SymbolicatorFailureMessageAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, "column must be uint32: 4294967296", attr.Str())
			},
		},
	}

	for _, tt := range tts {
		t.Run(tt.Name, func(t *testing.T) {
			s.clear()

			td := ptrace.NewTraces()
			rs := td.ResourceSpans().AppendEmpty()
			ils := rs.ScopeSpans().AppendEmpty()

			span := ils.Spans().AppendEmpty()
			span.SetName("first-batch-first-span")
			span.SetTraceID([16]byte{1, 2, 3, 4})

			tt.ApplyAttributes(span)

			otd, err := processor.processTraces(ctx, td)
			assert.NoError(t, err)

			tt.AssertSymbolicatorCalls(s)
			tt.AssertOutput(otd)
		})
	}
}
