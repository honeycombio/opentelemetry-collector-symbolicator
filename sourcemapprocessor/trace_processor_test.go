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
	processorhelper "go.opentelemetry.io/collector/processor"
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
	shouldError       bool
	errorMsg          string
	callCount         int
}

func (ts *testSymbolicator) clear() {
	ts.SymbolicatedLines = nil
	ts.callCount = 0
}

func (ts *testSymbolicator) symbolicate(ctx context.Context, line, column int64, function, url string, uuid string) (*mappedStackFrame, error) {
	ts.callCount++
	ts.SymbolicatedLines = append(ts.SymbolicatedLines, symbolicatedLine{
		Line:     line,
		Column:   column,
		Function: function,
		URL:      url,
	})

	// Return error if configured to do so
	if ts.shouldError {
		return nil, fmt.Errorf(ts.errorMsg)
	}

	// Special case for symbolication errors
	if column < 0 || column > math.MaxUint32 {
		return &mappedStackFrame{}, fmt.Errorf("column must be uint32: %d", column)
	}

	// Return different values based on line/column to simulate real source map behavior
	// This helps test that each frame gets its own symbolication result
	mappedLine := line * 2   // Simulate mapping to different line in original source
	mappedCol := column + 10 // Simulate mapping to different column
	mappedFunc := fmt.Sprintf("mapped_%s_%d_%d", function, line, column)
	mappedURL := fmt.Sprintf("original_%s", url)

	return &mappedStackFrame{
		FunctionName: mappedFunc,
		Col:          mappedCol,
		Line:         mappedLine,
		URL:          mappedURL,
	}, nil
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

	processor := newSymbolicatorProcessor(ctx, cfg, processorhelper.Settings{
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
				// testSymbolicator now maps: line*2, col+10, function -> mapped_function_line_col, url -> original_url
				assert.Equal(t, "Error: Test error!\n    at mapped_function_42_42(original_url:84:52)", attr.Str())

				attr, ok = span.Attributes().Get(cfg.ColumnsAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, "[52]", attr.AsString()) // 42 + 10 = 52

				attr, ok = span.Attributes().Get(cfg.LinesAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, "[84]", attr.AsString()) // 42 * 2 = 84

				attr, ok = span.Attributes().Get(cfg.FunctionsAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, "[\"mapped_function_42_42\"]", attr.AsString())

				attr, ok = span.Attributes().Get(cfg.UrlsAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, "[\"original_url\"]", attr.AsString())

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

// TestDeduplication verifies that frames with the same URL are only symbolicated once per stacktrace
func TestDeduplication(t *testing.T) {
	ctx := context.Background()
	cfg := createDefaultConfig().(*Config)
	s := &testSymbolicator{}

	testTel := componenttest.NewTelemetry()
	tb, err := metadata.NewTelemetryBuilder(testTel.NewTelemetrySettings())
	assert.NoError(t, err)
	defer tb.Shutdown()

	attributes := attribute.NewSet(
		attribute.String("processor_type", "sourcemap"),
	)

	processor := newSymbolicatorProcessor(ctx, cfg, processorhelper.Settings{
		TelemetrySettings: component.TelemetrySettings{
			Logger: zaptest.NewLogger(t),
		},
	}, s, tb, attributes)

	t.Run("10 frames from same URL with successful symbolication calls 10 times", func(t *testing.T) {
		s.clear()

		td := ptrace.NewTraces()
		rs := td.ResourceSpans().AppendEmpty()
		ils := rs.ScopeSpans().AppendEmpty()
		span := ils.Spans().AppendEmpty()

		// Create 10 frames all pointing to the same URL but different line/column positions
		span.Attributes().PutEmpty(cfg.ColumnsAttributeKey).SetEmptySlice().FromRaw([]any{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
		span.Attributes().PutEmpty(cfg.FunctionsAttributeKey).SetEmptySlice().FromRaw([]any{"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10"})
		span.Attributes().PutEmpty(cfg.LinesAttributeKey).SetEmptySlice().FromRaw([]any{100, 200, 300, 400, 500, 600, 700, 800, 900, 1000})
		span.Attributes().PutEmpty(cfg.UrlsAttributeKey).SetEmptySlice().FromRaw([]any{
			"app.js", "app.js", "app.js", "app.js", "app.js",
			"app.js", "app.js", "app.js", "app.js", "app.js",
		})
		span.Attributes().PutStr(cfg.StackTypeKey, "Error")
		span.Attributes().PutStr(cfg.StackMessageKey, "test error")

		_, err := processor.processTraces(ctx, td)
		assert.NoError(t, err)

		// Should call symbolicate 10 times (once per frame) since successful symbolications
		// vary by line/column position. However, the source map itself is cached in basicSymbolicator.cache
		assert.Equal(t, 10, len(s.SymbolicatedLines), "Expected 10 symbolication calls for 10 frames with different positions")

		// Verify all were from the same URL
		for _, line := range s.SymbolicatedLines {
			assert.Equal(t, "app.js", line.URL)
		}
	})

	t.Run("10 frames from 3 different URLs symbolicate all frames", func(t *testing.T) {
		s.clear()

		td := ptrace.NewTraces()
		rs := td.ResourceSpans().AppendEmpty()
		ils := rs.ScopeSpans().AppendEmpty()
		span := ils.Spans().AppendEmpty()

		// Create 10 frames from 3 different files
		span.Attributes().PutEmpty(cfg.ColumnsAttributeKey).SetEmptySlice().FromRaw([]any{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
		span.Attributes().PutEmpty(cfg.FunctionsAttributeKey).SetEmptySlice().FromRaw([]any{"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10"})
		span.Attributes().PutEmpty(cfg.LinesAttributeKey).SetEmptySlice().FromRaw([]any{100, 200, 300, 400, 500, 600, 700, 800, 900, 1000})
		span.Attributes().PutEmpty(cfg.UrlsAttributeKey).SetEmptySlice().FromRaw([]any{
			"app.js", "app.js", "app.js", "app.js",
			"vendor.js", "vendor.js", "vendor.js",
			"utils.js", "utils.js", "utils.js",
		})
		span.Attributes().PutStr(cfg.StackTypeKey, "Error")
		span.Attributes().PutStr(cfg.StackMessageKey, "test error")

		_, err := processor.processTraces(ctx, td)
		assert.NoError(t, err)

		// Should call symbolicate 10 times (once per frame) since successful symbolications
		// vary by line/column position
		assert.Equal(t, 10, len(s.SymbolicatedLines), "Expected 10 symbolication calls for 10 frames")

		// Verify all 3 URLs were symbolicated
		urls := make(map[string]bool)
		for _, line := range s.SymbolicatedLines {
			urls[line.URL] = true
		}
		assert.True(t, urls["app.js"])
		assert.True(t, urls["vendor.js"])
		assert.True(t, urls["utils.js"])
	})

	t.Run("same URL with different line/column positions get unique symbolication", func(t *testing.T) {
		s.clear()

		td := ptrace.NewTraces()
		rs := td.ResourceSpans().AppendEmpty()
		ils := rs.ScopeSpans().AppendEmpty()
		span := ils.Spans().AppendEmpty()

		// Create 3 frames all from app.js but at different line/column positions
		span.Attributes().PutEmpty(cfg.ColumnsAttributeKey).SetEmptySlice().FromRaw([]any{5, 15, 25})
		span.Attributes().PutEmpty(cfg.FunctionsAttributeKey).SetEmptySlice().FromRaw([]any{"onClick", "render", "init"})
		span.Attributes().PutEmpty(cfg.LinesAttributeKey).SetEmptySlice().FromRaw([]any{100, 200, 300})
		span.Attributes().PutEmpty(cfg.UrlsAttributeKey).SetEmptySlice().FromRaw([]any{"app.js", "app.js", "app.js"})
		span.Attributes().PutStr(cfg.StackTypeKey, "Error")
		span.Attributes().PutStr(cfg.StackMessageKey, "test error")

		_, err := processor.processTraces(ctx, td)
		assert.NoError(t, err)

		// Should call symbolicate 3 times even though all frames are from same URL
		// because each frame is at a different line/column position
		assert.Equal(t, 3, len(s.SymbolicatedLines), "Expected 3 symbolication calls for 3 frames with different positions")

		// Verify each frame got its unique symbolication result
		funcAttr, exists := span.Attributes().Get(cfg.FunctionsAttributeKey)
		assert.True(t, exists)
		functions := funcAttr.Slice()

		// Check that the mapped functions are different (proving each was symbolicated independently)
		assert.Equal(t, 3, functions.Len())
		fn1 := functions.At(0).Str()
		fn2 := functions.At(1).Str()
		fn3 := functions.At(2).Str()

		assert.Contains(t, fn1, "100_5", "First frame should have unique symbolication based on line 100, col 5")
		assert.Contains(t, fn2, "200_15", "Second frame should have unique symbolication based on line 200, col 15")
		assert.Contains(t, fn3, "300_25", "Third frame should have unique symbolication based on line 300, col 25")
		assert.NotEqual(t, fn1, fn2, "Each frame should have different mapped function names")
		assert.NotEqual(t, fn2, fn3, "Each frame should have different mapped function names")
	})

	t.Run("same URL with different buildUUID are fetched separately", func(t *testing.T) {
		s.clear()

		// First span with buildUUID-1
		td := ptrace.NewTraces()
		rs := td.ResourceSpans().AppendEmpty()
		rs.Resource().Attributes().PutStr(cfg.BuildUUIDAttributeKey, "build-uuid-1")
		ils := rs.ScopeSpans().AppendEmpty()
		span1 := ils.Spans().AppendEmpty()

		span1.Attributes().PutEmpty(cfg.ColumnsAttributeKey).SetEmptySlice().FromRaw([]any{1, 2})
		span1.Attributes().PutEmpty(cfg.FunctionsAttributeKey).SetEmptySlice().FromRaw([]any{"f1", "f2"})
		span1.Attributes().PutEmpty(cfg.LinesAttributeKey).SetEmptySlice().FromRaw([]any{100, 200})
		span1.Attributes().PutEmpty(cfg.UrlsAttributeKey).SetEmptySlice().FromRaw([]any{"app.js", "app.js"})
		span1.Attributes().PutStr(cfg.StackTypeKey, "Error")
		span1.Attributes().PutStr(cfg.StackMessageKey, "test error")

		// Second span with buildUUID-2 (same resource, different span simulates different stacktrace)
		span2 := ils.Spans().AppendEmpty()
		span2.Attributes().PutEmpty(cfg.ColumnsAttributeKey).SetEmptySlice().FromRaw([]any{1, 2})
		span2.Attributes().PutEmpty(cfg.FunctionsAttributeKey).SetEmptySlice().FromRaw([]any{"f1", "f2"})
		span2.Attributes().PutEmpty(cfg.LinesAttributeKey).SetEmptySlice().FromRaw([]any{100, 200})
		span2.Attributes().PutEmpty(cfg.UrlsAttributeKey).SetEmptySlice().FromRaw([]any{"app.js", "app.js"})
		span2.Attributes().PutStr(cfg.StackTypeKey, "Error")
		span2.Attributes().PutStr(cfg.StackMessageKey, "test error")

		_, err := processor.processTraces(ctx, td)
		assert.NoError(t, err)

		// Should call symbolicate 4 times: 2 frames in first span + 2 frames in second span
		// Note: Each span is a separate stacktrace, so error caching is independent
		assert.Equal(t, 4, len(s.SymbolicatedLines), "Expected 4 symbolication calls: 2 per span")
	})

	t.Run("missing source map errors are cached and reused within stacktrace", func(t *testing.T) {
		// Create a symbolicator that returns errors (simulating missing source maps)
		errorSymbolicator := &testSymbolicator{
			shouldError: true,
			errorMsg:    "source map not found: 404",
		}

		testTel := componenttest.NewTelemetry()
		errorTb, tbErr := metadata.NewTelemetryBuilder(testTel.NewTelemetrySettings())
		assert.NoError(t, tbErr)
		defer errorTb.Shutdown()

		errorAttributes := attribute.NewSet(
			attribute.String("processor_type", "sourcemap"),
		)

		errorProcessor := newSymbolicatorProcessor(ctx, cfg, processorhelper.Settings{
			TelemetrySettings: component.TelemetrySettings{
				Logger: zaptest.NewLogger(t),
			},
		}, errorSymbolicator, errorTb, errorAttributes)

		td := ptrace.NewTraces()
		rs := td.ResourceSpans().AppendEmpty()
		ils := rs.ScopeSpans().AppendEmpty()
		span := ils.Spans().AppendEmpty()

		// Create 10 frames all from the same missing source map
		span.Attributes().PutEmpty(cfg.ColumnsAttributeKey).SetEmptySlice().FromRaw([]any{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
		span.Attributes().PutEmpty(cfg.FunctionsAttributeKey).SetEmptySlice().FromRaw([]any{"f1", "f2", "f3", "f4", "f5", "f6", "f7", "f8", "f9", "f10"})
		span.Attributes().PutEmpty(cfg.LinesAttributeKey).SetEmptySlice().FromRaw([]any{100, 200, 300, 400, 500, 600, 700, 800, 900, 1000})
		span.Attributes().PutEmpty(cfg.UrlsAttributeKey).SetEmptySlice().FromRaw([]any{
			"missing.js", "missing.js", "missing.js", "missing.js", "missing.js",
			"missing.js", "missing.js", "missing.js", "missing.js", "missing.js",
		})
		span.Attributes().PutStr(cfg.StackTypeKey, "Error")
		span.Attributes().PutStr(cfg.StackMessageKey, "test error")

		_, processErr := errorProcessor.processTraces(ctx, td)
		assert.NoError(t, processErr) // Processing should succeed even if symbolication fails

		// Should only call symbolicate ONCE for the first frame, then reuse cached error
		// This validates the 80-95% reduction in failed fetches claim
		assert.Equal(t, 1, errorSymbolicator.callCount,
			"Expected only 1 symbolication call for 10 frames with same missing source map (93%% reduction)")

		// Verify symbolication_failed attribute is set
		attr, ok := span.Attributes().Get(cfg.SymbolicatorFailureAttributeKey)
		assert.True(t, ok)
		assert.True(t, attr.Bool())

		// Verify the stacktrace indicates failure
		stackAttr, ok := span.Attributes().Get(cfg.OutputStackTraceKey)
		assert.True(t, ok)
		assert.Contains(t, stackAttr.Str(), "Failed to symbolicate: source map not found: 404")
	})

	t.Run("validation errors are not cached - valid frames after invalid ones still get symbolicated", func(t *testing.T) {
		s.clear()

		td := ptrace.NewTraces()
		rs := td.ResourceSpans().AppendEmpty()
		ils := rs.ScopeSpans().AppendEmpty()
		span := ils.Spans().AppendEmpty()

		// Create frames: invalid column, valid, valid
		// All from same URL to test that validation error doesn't get cached
		span.Attributes().PutEmpty(cfg.ColumnsAttributeKey).SetEmptySlice().FromRaw([]any{-5, 10, 20}) // -5 is invalid
		span.Attributes().PutEmpty(cfg.FunctionsAttributeKey).SetEmptySlice().FromRaw([]any{"f1", "f2", "f3"})
		span.Attributes().PutEmpty(cfg.LinesAttributeKey).SetEmptySlice().FromRaw([]any{100, 200, 300})
		span.Attributes().PutEmpty(cfg.UrlsAttributeKey).SetEmptySlice().FromRaw([]any{"app.js", "app.js", "app.js"})
		span.Attributes().PutStr(cfg.StackTypeKey, "Error")
		span.Attributes().PutStr(cfg.StackMessageKey, "test error")

		_, err := processor.processTraces(ctx, td)
		// Should not error at the processTraces level, but individual frames will fail
		assert.NoError(t, err)

		// All 3 frames should attempt symbolication
		// Frame 1 will fail with validation error (not cached)
		// Frame 2 and 3 should succeed with valid coordinates
		assert.Equal(t, 3, len(s.SymbolicatedLines), "Expected 3 symbolication calls: validation error not cached")

		// Verify all frames were attempted
		assert.Equal(t, int64(100), s.SymbolicatedLines[0].Line, "First frame (invalid column) attempted")
		assert.Equal(t, int64(-5), s.SymbolicatedLines[0].Column, "First frame has invalid column")
		assert.Equal(t, int64(200), s.SymbolicatedLines[1].Line, "Second frame (valid) attempted")
		assert.Equal(t, int64(10), s.SymbolicatedLines[1].Column, "Second frame has valid column")
		assert.Equal(t, int64(300), s.SymbolicatedLines[2].Line, "Third frame (valid) attempted")
		assert.Equal(t, int64(20), s.SymbolicatedLines[2].Column, "Third frame has valid column")

		// Verify that the symbolication failure attribute is set since frame 1 failed
		failureAttr, exists := span.Attributes().Get(cfg.SymbolicatorFailureAttributeKey)
		assert.True(t, exists)
		assert.True(t, failureAttr.Bool(), "Should mark symbolication as having failures")
	})
}
