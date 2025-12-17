package sourcemapprocessor

import (
	"context"
	"errors"
	"fmt"
	"math"
	"testing"

	"github.com/honeycombio/opentelemetry-collector-symbolicator/sourcemapprocessor/internal/metadata"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/ptrace"
	processorhelper "go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric/noop"
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
	returnFetchError  bool // If true, wraps error in FetchError
	callCount         int
}

func (ts *testSymbolicator) clear() {
	ts.SymbolicatedLines = nil
	ts.callCount = 0
	ts.shouldError = false
	ts.returnFetchError = false
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
		err := errors.New(ts.errorMsg)
		if ts.returnFetchError {
			return nil, &FetchError{URL: url, Err: err}
		}
		return nil, err
	}

	// Special case for symbolication errors (validation)
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

func TestProcessTraces(t *testing.T) {
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
			Name: "skip processing for non-exception trace without StackTraceAttributeKey",
			ApplyAttributes: func(span ptrace.Span) {
				// Regular trace without any exception attributes
				span.Attributes().PutStr("http.method", "GET")
				span.Attributes().PutStr("http.url", "https://example.com/api/users")
				span.Attributes().PutInt("http.status_code", 200)
			},
			AssertSymbolicatorCalls: func(s *testSymbolicator) {
				// Should NOT call symbolicate at all
				assert.Empty(t, s.SymbolicatedLines)
			},
			AssertOutput: func(td ptrace.Traces) {
				rs := td.ResourceSpans().At(0)
				ils := rs.ScopeSpans().At(0)
				span := ils.Spans().At(0)

				// Verify processor type and version attributes are NOT set
				_, ok := span.Attributes().Get("honeycomb.processor_type")
				assert.False(t, ok)

				_, ok = span.Attributes().Get("honeycomb.processor_version")
				assert.False(t, ok)

				// Verify symbolicator failure attributes are NOT set
				_, ok = span.Attributes().Get(cfg.SymbolicatorFailureAttributeKey)
				assert.False(t, ok)

				// Verify original attributes are preserved
				attr, ok := span.Attributes().Get("http.method")
				assert.True(t, ok)
				assert.Equal(t, "GET", attr.Str())
			},
		},
		{
			Name: "symbolicated stacktrace attribute provided",
			ApplyAttributes: func(span ptrace.Span) {
				span.Attributes().PutEmpty(cfg.ColumnsAttributeKey).SetEmptySlice().AppendEmpty().SetInt(42)
				span.Attributes().PutEmpty(cfg.LinesAttributeKey).SetEmptySlice().AppendEmpty().SetInt(42)
				span.Attributes().PutEmpty(cfg.FunctionsAttributeKey).SetEmptySlice().AppendEmpty().SetStr("function")
				span.Attributes().PutEmpty(cfg.UrlsAttributeKey).SetEmptySlice().AppendEmpty().SetStr("url")
				span.Attributes().PutEmpty(cfg.ExceptionTypeAttributeKey).SetStr("Error")
				span.Attributes().PutEmpty(cfg.ExceptionMessageAttributeKey).SetStr("Test error!")
				span.Attributes().PutStr(cfg.StackTraceAttributeKey, "Error: Test error!\n    at function (url:42:42)")
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

				attr, ok := span.Attributes().Get(cfg.StackTraceAttributeKey)
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

				_, ok = span.Attributes().Get(cfg.SymbolicatorErrorAttributeKey)
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
				span.Attributes().PutEmpty(cfg.StackTraceAttributeKey).SetStr("Error: test error\n    at func1 (url1:4:1)\n    at func2 (url2:5:2)\n    at func3 (url3:6:3)")
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

				attr, ok = span.Attributes().Get(cfg.OriginalStackTraceAttributeKey)
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
				span.Attributes().PutEmpty(cfg.StackTraceAttributeKey).SetStr("Error: test error\n    at func1 (url1:4:1)\n    at func2 (url2:5:2)\n    at func3 (url3:6:3)")
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

				_, ok = span.Attributes().Get(cfg.OriginalStackTraceAttributeKey)
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
				span.Attributes().PutEmpty(cfg.StackTraceAttributeKey).SetStr("Error: test error\n    at func1 (url1:4:1)\n    at func2 (url2:5:5000000000)\n    at func3 (url3:6:3)")
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
				attr, ok = span.Attributes().Get(cfg.SymbolicatorErrorAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, "symbolication failed for some stack frames", attr.Str())
				attr, ok = span.Attributes().Get(cfg.StackTraceAttributeKey)
				assert.True(t, ok)
				assert.Contains(t, attr.AsString(), "Failed to symbolicate func2 at url2:5:4294967296")
				assert.Contains(t, attr.AsString(), "column must be uint32: 4294967296")
			},
		},
		{
			Name: "processor parses raw stack trace when structured attributes missing",
			ApplyAttributes: func(span ptrace.Span) {
				// Only provide raw stack trace - no structured attributes
				span.Attributes().PutStr(cfg.ExceptionTypeAttributeKey, "Error")
				span.Attributes().PutStr(cfg.ExceptionMessageAttributeKey, "Test error!")
				span.Attributes().PutStr(
					cfg.StackTraceAttributeKey,
					"Error: Test error!\n"+
						"    at myFunction (https://example.com/app.js:10:15)\n"+
						"    at anotherFunc (https://example.com/app.js:20:25)",
				)
			},
			AssertSymbolicatorCalls: func(s *testSymbolicator) {
				assert.Equal(t, 2, len(s.SymbolicatedLines))
				// First frame
				assert.Equal(t, int64(10), s.SymbolicatedLines[0].Line)
				assert.Equal(t, int64(15), s.SymbolicatedLines[0].Column)
				assert.Equal(t, "myFunction", s.SymbolicatedLines[0].Function)
				assert.Equal(t, "https://example.com/app.js", s.SymbolicatedLines[0].URL)
				// Second frame
				assert.Equal(t, int64(20), s.SymbolicatedLines[1].Line)
				assert.Equal(t, int64(25), s.SymbolicatedLines[1].Column)
				assert.Equal(t, "anotherFunc", s.SymbolicatedLines[1].Function)
				assert.Equal(t, "https://example.com/app.js", s.SymbolicatedLines[1].URL)
			},
			AssertOutput: func(td ptrace.Traces) {
				rs := td.ResourceSpans().At(0)
				ils := rs.ScopeSpans().At(0)
				span := ils.Spans().At(0)

				// Verify parsing method attribute is set
				parsingMethod, ok := span.Attributes().Get(cfg.SymbolicatorParsingMethodAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, "processor_parsed", parsingMethod.Str())

				// Verify symbolication succeeded
				attr, ok := span.Attributes().Get(cfg.SymbolicatorFailureAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, false, attr.Bool())

				// Verify stack trace was symbolicated with both frames
				stackTrace, ok := span.Attributes().Get(cfg.StackTraceAttributeKey)
				assert.True(t, ok)
				assert.Contains(t, stackTrace.Str(), "Error: Test error!")
				// First frame symbolicated: line*2=20, col+10=25
				assert.Contains(t, stackTrace.Str(), "mapped_myFunction_10_15")
				assert.Contains(t, stackTrace.Str(), "original_https://example.com/app.js:20:25")
				// Second frame symbolicated: line*2=40, col+10=35
				assert.Contains(t, stackTrace.Str(), "mapped_anotherFunc_20_25")
				assert.Contains(t, stackTrace.Str(), "original_https://example.com/app.js:40:35")
			},
		},
		{
			Name: "span events with exception stacktrace are symbolicated",
			ApplyAttributes: func(span ptrace.Span) {
				// Add a regular span attribute (not an exception)
				span.Attributes().PutStr("http.method", "GET")

				// Add an exception event following OTel semantic conventions
				event := span.Events().AppendEmpty()
				event.SetName("exception")
				event.Attributes().PutEmpty(cfg.ColumnsAttributeKey).SetEmptySlice().AppendEmpty().SetInt(15)
				event.Attributes().PutEmpty(cfg.LinesAttributeKey).SetEmptySlice().AppendEmpty().SetInt(10)
				event.Attributes().PutEmpty(cfg.FunctionsAttributeKey).SetEmptySlice().AppendEmpty().SetStr("eventFunction")
				event.Attributes().PutEmpty(cfg.UrlsAttributeKey).SetEmptySlice().AppendEmpty().SetStr("https://example.com/event.js")
				event.Attributes().PutStr(cfg.ExceptionTypeAttributeKey, "Error")
				event.Attributes().PutStr(cfg.ExceptionMessageAttributeKey, "Event error!")
				event.Attributes().PutStr(cfg.StackTraceAttributeKey, "Error: Event error!\n    at eventFunction (https://example.com/event.js:10:15)")
			},
			AssertSymbolicatorCalls: func(s *testSymbolicator) {
				// Should symbolicate the event's stacktrace
				assert.ElementsMatch(t, s.SymbolicatedLines, []symbolicatedLine{
					{Line: 10, Column: 15, Function: "eventFunction", URL: "https://example.com/event.js"},
				})
			},
			AssertOutput: func(td ptrace.Traces) {
				rs := td.ResourceSpans().At(0)
				ils := rs.ScopeSpans().At(0)
				span := ils.Spans().At(0)

				// Verify span attributes are not modified (no exception on span itself)
				attr, ok := span.Attributes().Get("http.method")
				assert.True(t, ok)
				assert.Equal(t, "GET", attr.Str())

				// Verify span doesn't have processor attributes (exception was on event)
				_, ok = span.Attributes().Get(cfg.StackTraceAttributeKey)
				assert.False(t, ok)

				// Verify event was symbolicated
				assert.Equal(t, 1, span.Events().Len())
				event := span.Events().At(0)
				assert.Equal(t, "exception", event.Name())

				// Verify event's stacktrace was symbolicated
				stackTrace, ok := event.Attributes().Get(cfg.StackTraceAttributeKey)
				assert.True(t, ok)
				assert.Contains(t, stackTrace.Str(), "Error: Event error!")
				assert.Contains(t, stackTrace.Str(), "mapped_eventFunction_10_15")
				assert.Contains(t, stackTrace.Str(), "original_https://example.com/event.js:20:25")

				// Verify processor attributes are on the event
				processorType, ok := event.Attributes().Get("honeycomb.processor_type")
				assert.True(t, ok)
				assert.Equal(t, typeStr.String(), processorType.Str())
			},
		},
		{
			Name: "span events with raw stacktrace (no structured attributes) are parsed and symbolicated",
			ApplyAttributes: func(span ptrace.Span) {
				// Add a regular span attribute (not an exception)
				span.Attributes().PutStr("http.method", "POST")

				// Add an exception event with only raw stacktrace (no structured attributes)
				event := span.Events().AppendEmpty()
				event.SetName("exception")
				event.Attributes().PutStr(cfg.ExceptionTypeAttributeKey, "TypeError")
				event.Attributes().PutStr(cfg.ExceptionMessageAttributeKey, "Cannot read property 'foo' of undefined")
				event.Attributes().PutStr(cfg.StackTraceAttributeKey,
					"TypeError: Cannot read property 'foo' of undefined\n"+
						"    at processData (https://example.com/bundle.js:1:5000)\n"+
						"    at handleClick (https://example.com/bundle.js:1:3000)")
			},
			AssertSymbolicatorCalls: func(s *testSymbolicator) {
				// Should symbolicate both frames from parsed raw stacktrace
				assert.Len(t, s.SymbolicatedLines, 2)
				assert.Equal(t, symbolicatedLine{Line: 1, Column: 5000, Function: "processData", URL: "https://example.com/bundle.js"}, s.SymbolicatedLines[0])
				assert.Equal(t, symbolicatedLine{Line: 1, Column: 3000, Function: "handleClick", URL: "https://example.com/bundle.js"}, s.SymbolicatedLines[1])
			},
			AssertOutput: func(td ptrace.Traces) {
				rs := td.ResourceSpans().At(0)
				ils := rs.ScopeSpans().At(0)
				span := ils.Spans().At(0)

				// Verify span attributes are not modified
				attr, ok := span.Attributes().Get("http.method")
				assert.True(t, ok)
				assert.Equal(t, "POST", attr.Str())

				// Verify event was parsed and symbolicated
				assert.Equal(t, 1, span.Events().Len())
				event := span.Events().At(0)
				assert.Equal(t, "exception", event.Name())

				// Verify event's stacktrace was symbolicated
				stackTrace, ok := event.Attributes().Get(cfg.StackTraceAttributeKey)
				assert.True(t, ok)
				assert.Contains(t, stackTrace.Str(), "TypeError: Cannot read property 'foo' of undefined")
				assert.Contains(t, stackTrace.Str(), "mapped_processData_1_5000")
				assert.Contains(t, stackTrace.Str(), "original_https://example.com/bundle.js:2:5010")
				assert.Contains(t, stackTrace.Str(), "mapped_handleClick_1_3000")
				assert.Contains(t, stackTrace.Str(), "original_https://example.com/bundle.js:2:3010")

				// Verify parsing method is processor_parsed
				parsingMethod, ok := event.Attributes().Get(cfg.SymbolicatorParsingMethodAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, "processor_parsed", parsingMethod.Str())
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

func TestProcessLogs(t *testing.T) {
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
		ApplyAttributes         func(logRecord plog.LogRecord)
		AssertSymbolicatorCalls func(s *testSymbolicator)
		AssertOutput            func(logs plog.Logs)
	}{
		{
			Name: "skip processing for non-exception log without StackTraceAttributeKey",
			ApplyAttributes: func(logRecord plog.LogRecord) {
				// Regular log without any exception attributes
				logRecord.Attributes().PutStr("level", "info")
				logRecord.Attributes().PutStr("message", "User logged in successfully")
				logRecord.Attributes().PutStr("user.id", "12345")
			},
			AssertSymbolicatorCalls: func(s *testSymbolicator) {
				// Should NOT call symbolicate at all
				assert.Empty(t, s.SymbolicatedLines)
			},
			AssertOutput: func(logs plog.Logs) {
				rl := logs.ResourceLogs().At(0)
				sl := rl.ScopeLogs().At(0)
				logRecord := sl.LogRecords().At(0)

				// Verify processor type and version attributes are NOT set
				_, ok := logRecord.Attributes().Get("honeycomb.processor_type")
				assert.False(t, ok)

				_, ok = logRecord.Attributes().Get("honeycomb.processor_version")
				assert.False(t, ok)

				// Verify symbolicator failure attributes are NOT set
				_, ok = logRecord.Attributes().Get(cfg.SymbolicatorFailureAttributeKey)
				assert.False(t, ok)

				// Verify original attributes are preserved
				attr, ok := logRecord.Attributes().Get("message")
				assert.True(t, ok)
				assert.Equal(t, "User logged in successfully", attr.Str())
			},
		},
		{
			Name: "symbolicated stacktrace attribute provided",
			ApplyAttributes: func(logRecord plog.LogRecord) {
				logRecord.Attributes().PutEmpty(cfg.ColumnsAttributeKey).SetEmptySlice().AppendEmpty().SetInt(42)
				logRecord.Attributes().PutEmpty(cfg.LinesAttributeKey).SetEmptySlice().AppendEmpty().SetInt(42)
				logRecord.Attributes().PutEmpty(cfg.FunctionsAttributeKey).SetEmptySlice().AppendEmpty().SetStr("function")
				logRecord.Attributes().PutEmpty(cfg.UrlsAttributeKey).SetEmptySlice().AppendEmpty().SetStr("url")
				logRecord.Attributes().PutEmpty(cfg.ExceptionTypeAttributeKey).SetStr("Error")
				logRecord.Attributes().PutEmpty(cfg.ExceptionMessageAttributeKey).SetStr("Test error!")
				logRecord.Attributes().PutStr(cfg.StackTraceAttributeKey, "Error: Test error!\n    at function (url:42:42)")
			},
			AssertSymbolicatorCalls: func(s *testSymbolicator) {
				assert.ElementsMatch(t, s.SymbolicatedLines, []symbolicatedLine{
					{Line: 42, Column: 42, Function: "function", URL: "url"},
				})
			},
			AssertOutput: func(logs plog.Logs) {
				rl := logs.ResourceLogs().At(0)
				sl := rl.ScopeLogs().At(0)
				logRecord := sl.LogRecords().At(0)

				// Verify processor type and version attributes are included
				processorTypeAttr, ok := logRecord.Attributes().Get("honeycomb.processor_type")
				assert.True(t, ok)
				assert.Equal(t, typeStr.String(), processorTypeAttr.Str())

				processorVersionAttr, ok := logRecord.Attributes().Get("honeycomb.processor_version")
				assert.True(t, ok)
				assert.Equal(t, processorVersion, processorVersionAttr.Str())

				attr, ok := logRecord.Attributes().Get(cfg.StackTraceAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, "Error: Test error!\n    at mapped_function_42_42(original_url:84:52)", attr.Str())

				attr, ok = logRecord.Attributes().Get(cfg.SymbolicatorFailureAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, false, attr.Bool())
			},
		},
		{
			Name: "multiple frames symbolicated successfully",
			ApplyAttributes: func(logRecord plog.LogRecord) {
				logRecord.Attributes().PutEmpty(cfg.ColumnsAttributeKey).SetEmptySlice().FromRaw([]any{1, 2, 3})
				logRecord.Attributes().PutEmpty(cfg.LinesAttributeKey).SetEmptySlice().FromRaw([]any{4, 5, 6})
				logRecord.Attributes().PutEmpty(cfg.FunctionsAttributeKey).SetEmptySlice().FromRaw([]any{"func1", "func2", "func3"})
				logRecord.Attributes().PutEmpty(cfg.UrlsAttributeKey).SetEmptySlice().FromRaw([]any{"url1", "url2", "url3"})
				logRecord.Attributes().PutEmpty(cfg.ExceptionTypeAttributeKey).SetStr("Error")
				logRecord.Attributes().PutEmpty(cfg.ExceptionMessageAttributeKey).SetStr("test error")
				logRecord.Attributes().PutStr(cfg.StackTraceAttributeKey, "Error: test error\n    at func1 (url1:4:1)\n    at func2 (url2:5:2)\n    at func3 (url3:6:3)")
			},
			AssertSymbolicatorCalls: func(s *testSymbolicator) {
				assert.ElementsMatch(t, s.SymbolicatedLines, []symbolicatedLine{
					{Line: 4, Column: 1, Function: "func1", URL: "url1"},
					{Line: 5, Column: 2, Function: "func2", URL: "url2"},
					{Line: 6, Column: 3, Function: "func3", URL: "url3"},
				})
			},
			AssertOutput: func(logs plog.Logs) {
				rl := logs.ResourceLogs().At(0)
				sl := rl.ScopeLogs().At(0)
				logRecord := sl.LogRecords().At(0)

				attr, ok := logRecord.Attributes().Get(cfg.ColumnsAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, "[11,12,13]", attr.AsString())

				attr, ok = logRecord.Attributes().Get(cfg.LinesAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, "[8,10,12]", attr.AsString())

				attr, ok = logRecord.Attributes().Get(cfg.FunctionsAttributeKey)
				assert.True(t, ok)
				assert.Contains(t, attr.AsString(), "mapped_func1_4_1")
				assert.Contains(t, attr.AsString(), "mapped_func2_5_2")
				assert.Contains(t, attr.AsString(), "mapped_func3_6_3")

				attr, ok = logRecord.Attributes().Get(cfg.SymbolicatorFailureAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, false, attr.Bool())
			},
		},
		{
			Name: "symbolication failed attribute set to true on symbolication error",
			ApplyAttributes: func(logRecord plog.LogRecord) {
				logRecord.Attributes().PutEmpty(cfg.ColumnsAttributeKey).SetEmptySlice().FromRaw([]any{1, int64(math.MaxUint32) + 1, 3})
				logRecord.Attributes().PutEmpty(cfg.LinesAttributeKey).SetEmptySlice().FromRaw([]any{4, 5, 6})
				logRecord.Attributes().PutEmpty(cfg.FunctionsAttributeKey).SetEmptySlice().FromRaw([]any{"func1", "func2", "func3"})
				logRecord.Attributes().PutEmpty(cfg.UrlsAttributeKey).SetEmptySlice().FromRaw([]any{"url1", "url2", "url3"})
				logRecord.Attributes().PutEmpty(cfg.ExceptionTypeAttributeKey).SetStr("Error")
				logRecord.Attributes().PutEmpty(cfg.ExceptionMessageAttributeKey).SetStr("test error")
				logRecord.Attributes().PutStr(cfg.StackTraceAttributeKey, "Error: test error\n    at func1 (url1:4:1)\n    at func2 (url2:5:5000000000)\n    at func3 (url3:6:3)")
			},
			AssertSymbolicatorCalls: func(s *testSymbolicator) {
				assert.ElementsMatch(t, s.SymbolicatedLines, []symbolicatedLine{
					{Line: 4, Column: 1, Function: "func1", URL: "url1"},
					{Line: 5, Column: int64(math.MaxUint32) + 1, Function: "func2", URL: "url2"},
					{Line: 6, Column: 3, Function: "func3", URL: "url3"},
				})
			},
			AssertOutput: func(logs plog.Logs) {
				rl := logs.ResourceLogs().At(0)
				sl := rl.ScopeLogs().At(0)
				logRecord := sl.LogRecords().At(0)

				// Verify processor type and version attributes are included even on failure
				processorTypeAttr, ok := logRecord.Attributes().Get("honeycomb.processor_type")
				assert.True(t, ok)
				assert.Equal(t, typeStr.String(), processorTypeAttr.Str())

				processorVersionAttr, ok := logRecord.Attributes().Get("honeycomb.processor_version")
				assert.True(t, ok)
				assert.Equal(t, processorVersion, processorVersionAttr.Str())

				// Verify failure attributes
				attr, ok := logRecord.Attributes().Get(cfg.SymbolicatorFailureAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, true, attr.Bool())

				attr, ok = logRecord.Attributes().Get(cfg.SymbolicatorErrorAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, "symbolication failed for some stack frames", attr.Str())
				attr, ok = logRecord.Attributes().Get(cfg.StackTraceAttributeKey)
				assert.True(t, ok)
				assert.Contains(t, attr.AsString(), "Failed to symbolicate func2 at url2:5:4294967296")
				assert.Contains(t, attr.AsString(), "column must be uint32: 4294967296")
			},
		},
		{
			Name: "processor parses raw stack trace when structured attributes missing",
			ApplyAttributes: func(logRecord plog.LogRecord) {
				// Only provide raw stack trace - no structured attributes
				logRecord.Attributes().PutStr(cfg.ExceptionTypeAttributeKey, "TypeError")
				logRecord.Attributes().PutStr(cfg.ExceptionMessageAttributeKey, "Cannot read property 'x'")
				logRecord.Attributes().PutStr(
					cfg.StackTraceAttributeKey,
					"TypeError: Cannot read property 'x'\n"+
						"    at processData (https://example.com/bundle.js:100:50)\n"+
						"    at main (https://example.com/bundle.js:200:30)",
				)
			},
			AssertSymbolicatorCalls: func(s *testSymbolicator) {
				assert.Equal(t, 2, len(s.SymbolicatedLines))
				// First frame
				assert.Equal(t, int64(100), s.SymbolicatedLines[0].Line)
				assert.Equal(t, int64(50), s.SymbolicatedLines[0].Column)
				assert.Equal(t, "processData", s.SymbolicatedLines[0].Function)
				assert.Equal(t, "https://example.com/bundle.js", s.SymbolicatedLines[0].URL)
				// Second frame
				assert.Equal(t, int64(200), s.SymbolicatedLines[1].Line)
				assert.Equal(t, int64(30), s.SymbolicatedLines[1].Column)
				assert.Equal(t, "main", s.SymbolicatedLines[1].Function)
				assert.Equal(t, "https://example.com/bundle.js", s.SymbolicatedLines[1].URL)
			},
			AssertOutput: func(logs plog.Logs) {
				rl := logs.ResourceLogs().At(0)
				sl := rl.ScopeLogs().At(0)
				logRecord := sl.LogRecords().At(0)

				// Verify parsing method attribute is set
				parsingMethod, ok := logRecord.Attributes().Get(cfg.SymbolicatorParsingMethodAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, "processor_parsed", parsingMethod.Str())

				// Verify symbolication succeeded
				attr, ok := logRecord.Attributes().Get(cfg.SymbolicatorFailureAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, false, attr.Bool())

				// Verify stack trace was symbolicated with both frames
				stackTrace, ok := logRecord.Attributes().Get(cfg.StackTraceAttributeKey)
				assert.True(t, ok)
				assert.Contains(t, stackTrace.Str(), "TypeError: Cannot read property 'x'")
				// First frame symbolicated: line*2=200, col+10=60
				assert.Contains(t, stackTrace.Str(), "mapped_processData_100_50")
				assert.Contains(t, stackTrace.Str(), "original_https://example.com/bundle.js:200:60")
				// Second frame symbolicated: line*2=400, col+10=40
				assert.Contains(t, stackTrace.Str(), "mapped_main_200_30")
				assert.Contains(t, stackTrace.Str(), "original_https://example.com/bundle.js:400:40")
			},
		},
		{
			Name: "native frames with empty URL are not symbolicated",
			ApplyAttributes: func(logRecord plog.LogRecord) {
				logRecord.Attributes().PutStr(cfg.ExceptionTypeAttributeKey, "Error")
				logRecord.Attributes().PutStr(cfg.ExceptionMessageAttributeKey, "test")
				logRecord.Attributes().PutStr(cfg.StackTraceAttributeKey, "Error: test\n    at Array.forEach (native)\n    at funcA (http://example.com/bundle.js:10:5)\n    at Array.map (native)")
			},
			AssertSymbolicatorCalls: func(s *testSymbolicator) {
				// Only the non-native frame should be symbolicated
				assert.Len(t, s.SymbolicatedLines, 1)
				assert.Equal(t, symbolicatedLine{Line: 10, Column: 5, Function: "funcA", URL: "http://example.com/bundle.js"}, s.SymbolicatedLines[0])
			},
			AssertOutput: func(logs plog.Logs) {
				rl := logs.ResourceLogs().At(0)
				sl := rl.ScopeLogs().At(0)
				logRecord := sl.LogRecords().At(0)

				stackTrace, ok := logRecord.Attributes().Get(cfg.StackTraceAttributeKey)
				assert.True(t, ok)
				// Native frames should be preserved as-is
				assert.Contains(t, stackTrace.Str(), "at Array.forEach (native)")
				assert.Contains(t, stackTrace.Str(), "at Array.map (native)")
				// Regular frame should be symbolicated
				assert.Contains(t, stackTrace.Str(), "mapped_funcA_10_5")
			},
		},
		{
			Name: "native frames with [native code] URL are not symbolicated",
			ApplyAttributes: func(logRecord plog.LogRecord) {
				// Safari/Firefox native frames have [native code] as URL
				// Parser converts this to empty URL, so they're skipped
				logRecord.Attributes().PutStr(cfg.ExceptionTypeAttributeKey, "Error")
				logRecord.Attributes().PutStr(cfg.ExceptionMessageAttributeKey, "test")
				logRecord.Attributes().PutStr(cfg.StackTraceAttributeKey, "Error: test\neval@[native code]\nfoo@http://example.com/bundle.js:10:5")
			},
			AssertSymbolicatorCalls: func(s *testSymbolicator) {
				// Only the non-native frame should be symbolicated
				assert.Len(t, s.SymbolicatedLines, 1)
				assert.Equal(t, symbolicatedLine{Line: 10, Column: 5, Function: "foo", URL: "http://example.com/bundle.js"}, s.SymbolicatedLines[0])
			},
			AssertOutput: func(logs plog.Logs) {
				rl := logs.ResourceLogs().At(0)
				sl := rl.ScopeLogs().At(0)
				logRecord := sl.LogRecords().At(0)

				stackTrace, ok := logRecord.Attributes().Get(cfg.StackTraceAttributeKey)
				assert.True(t, ok)
				// Native frame should be preserved as-is
				assert.Contains(t, stackTrace.Str(), "at eval (native)")
				// Regular frame should be symbolicated
				assert.Contains(t, stackTrace.Str(), "mapped_foo_10_5")
			},
		},
		{
			Name: "React Native stacktrace with native frames",
			ApplyAttributes: func(logRecord plog.LogRecord) {
				// React Native format with "address at" and native frames
				logRecord.Attributes().PutStr(cfg.ExceptionTypeAttributeKey, "Error")
				logRecord.Attributes().PutStr(cfg.ExceptionMessageAttributeKey, "test")
				logRecord.Attributes().PutStr(cfg.StackTraceAttributeKey,
					"Error: test\n"+
						"    at anonymous (address at index.android.bundle:1:2347115)\n"+
						"    at call (native)\n"+
						"    at apply (native)\n"+
						"    at _with (address at index.android.bundle:1:1414154)")
			},
			AssertSymbolicatorCalls: func(s *testSymbolicator) {
				// Only the non-native frames should be symbolicated (2 frames)
				assert.Len(t, s.SymbolicatedLines, 2)
				assert.Equal(t, symbolicatedLine{Line: 1, Column: 2347115, Function: "anonymous", URL: "index.android.bundle"}, s.SymbolicatedLines[0])
				assert.Equal(t, symbolicatedLine{Line: 1, Column: 1414154, Function: "_with", URL: "index.android.bundle"}, s.SymbolicatedLines[1])
			},
			AssertOutput: func(logs plog.Logs) {
				rl := logs.ResourceLogs().At(0)
				sl := rl.ScopeLogs().At(0)
				logRecord := sl.LogRecords().At(0)

				stackTrace, ok := logRecord.Attributes().Get(cfg.StackTraceAttributeKey)
				assert.True(t, ok)
				// Native frames should be preserved
				assert.Contains(t, stackTrace.Str(), "at call (native)")
				assert.Contains(t, stackTrace.Str(), "at apply (native)")
				// Regular frames should be symbolicated
				assert.Contains(t, stackTrace.Str(), "mapped_anonymous_1_2347115")
				assert.Contains(t, stackTrace.Str(), "mapped__with_1_1414154")

				// Verify parsing method is processor_parsed (due to "address at")
				parsingMethod, ok := logRecord.Attributes().Get(cfg.SymbolicatorParsingMethodAttributeKey)
				assert.True(t, ok)
				assert.Equal(t, "processor_parsed", parsingMethod.Str())
			},
		},
		{
			Name: "frames with anonymous urls are not symbolicated",
			ApplyAttributes: func(logRecord plog.LogRecord) {
				logRecord.Attributes().PutStr(cfg.ExceptionTypeAttributeKey, "Error")
				logRecord.Attributes().PutStr(cfg.ExceptionMessageAttributeKey, "test error")
				logRecord.Attributes().PutStr(cfg.StackTraceAttributeKey, "Error: test error\n    at JSON.parse (<anonymous>)\n    at foo (http://example.com/bundle.js:10:5)")
			},
			AssertSymbolicatorCalls: func(s *testSymbolicator) {
				// Only the non-anonymous frame should be symbolicated
				assert.Len(t, s.SymbolicatedLines, 1)
				assert.Equal(t, symbolicatedLine{Line: 10, Column: 5, Function: "foo", URL: "http://example.com/bundle.js"}, s.SymbolicatedLines[0])
			},
			AssertOutput: func(logs plog.Logs) {
				rl := logs.ResourceLogs().At(0)
				sl := rl.ScopeLogs().At(0)
				logRecord := sl.LogRecords().At(0)

				stackTrace, ok := logRecord.Attributes().Get(cfg.StackTraceAttributeKey)
				assert.True(t, ok)
				// Anonymous frame should be preserved with <anonymous>
				assert.Contains(t, stackTrace.Str(), "at JSON.parse (<anonymous>)")
				// Regular frame should be symbolicated
				assert.Contains(t, stackTrace.Str(), "mapped_foo_10_5")
			},
		},
	}

	for _, tt := range tts {
		t.Run(tt.Name, func(t *testing.T) {
			s.clear()

			logs := plog.NewLogs()
			rl := logs.ResourceLogs().AppendEmpty()
			sl := rl.ScopeLogs().AppendEmpty()

			logRecord := sl.LogRecords().AppendEmpty()
			logRecord.SetTimestamp(1234567890)

			tt.ApplyAttributes(logRecord)

			outputLogs, err := processor.processLogs(ctx, logs)
			assert.NoError(t, err)

			tt.AssertSymbolicatorCalls(s)
			tt.AssertOutput(outputLogs)
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
		span.Attributes().PutStr(cfg.ExceptionTypeAttributeKey, "Error")
		span.Attributes().PutStr(cfg.ExceptionMessageAttributeKey, "test error")
		span.Attributes().PutStr(cfg.StackTraceAttributeKey,
			"Error: test error\n"+
				"    at f1 (app.js:100:1)\n"+
				"    at f2 (app.js:200:2)\n"+
				"    at f3 (app.js:300:3)\n"+
				"    at f4 (app.js:400:4)\n"+
				"    at f5 (app.js:500:5)\n"+
				"    at f6 (app.js:600:6)\n"+
				"    at f7 (app.js:700:7)\n"+
				"    at f8 (app.js:800:8)\n"+
				"    at f9 (app.js:900:9)\n"+
				"    at f10 (app.js:1000:10)")

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
		span.Attributes().PutStr(cfg.ExceptionTypeAttributeKey, "Error")
		span.Attributes().PutStr(cfg.ExceptionMessageAttributeKey, "test error")
		span.Attributes().PutStr(cfg.StackTraceAttributeKey,
			"Error: test error\n"+
				"    at f1 (app.js:100:1)\n"+
				"    at f2 (app.js:200:2)\n"+
				"    at f3 (app.js:300:3)\n"+
				"    at f4 (app.js:400:4)\n"+
				"    at f5 (vendor.js:500:5)\n"+
				"    at f6 (vendor.js:600:6)\n"+
				"    at f7 (vendor.js:700:7)\n"+
				"    at f8 (utils.js:800:8)\n"+
				"    at f9 (utils.js:900:9)\n"+
				"    at f10 (utils.js:1000:10)")

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
		span.Attributes().PutStr(cfg.ExceptionTypeAttributeKey, "Error")
		span.Attributes().PutStr(cfg.ExceptionMessageAttributeKey, "test error")
		span.Attributes().PutStr(cfg.StackTraceAttributeKey,
			"Error: test error\n"+
				"    at onClick (app.js:100:5)\n"+
				"    at render (app.js:200:15)\n"+
				"    at init (app.js:300:25)")

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
		span1.Attributes().PutStr(cfg.ExceptionTypeAttributeKey, "Error")
		span1.Attributes().PutStr(cfg.ExceptionMessageAttributeKey, "test error")
		span1.Attributes().PutStr(cfg.StackTraceAttributeKey,
			"Error: test error\n"+
				"    at f1 (app.js:100:1)\n"+
				"    at f2 (app.js:200:2)")

		// Second span with buildUUID-2 (same resource, different span simulates different stacktrace)
		span2 := ils.Spans().AppendEmpty()
		span2.Attributes().PutEmpty(cfg.ColumnsAttributeKey).SetEmptySlice().FromRaw([]any{1, 2})
		span2.Attributes().PutEmpty(cfg.FunctionsAttributeKey).SetEmptySlice().FromRaw([]any{"f1", "f2"})
		span2.Attributes().PutEmpty(cfg.LinesAttributeKey).SetEmptySlice().FromRaw([]any{100, 200})
		span2.Attributes().PutEmpty(cfg.UrlsAttributeKey).SetEmptySlice().FromRaw([]any{"app.js", "app.js"})
		span2.Attributes().PutStr(cfg.ExceptionTypeAttributeKey, "Error")
		span2.Attributes().PutStr(cfg.ExceptionMessageAttributeKey, "test error")
		span2.Attributes().PutStr(cfg.StackTraceAttributeKey,
			"Error: test error\n"+
				"    at f1 (app.js:100:1)\n"+
				"    at f2 (app.js:200:2)")

		_, err := processor.processTraces(ctx, td)
		assert.NoError(t, err)

		// Should call symbolicate 4 times: 2 frames in first span + 2 frames in second span
		// Note: Each span is a separate stacktrace, so error caching is independent
		assert.Equal(t, 4, len(s.SymbolicatedLines), "Expected 4 symbolication calls: 2 per span")
	})

	t.Run("missing source map errors are cached and reused within stacktrace", func(t *testing.T) {
		// Create a symbolicator that returns FetchError (simulating missing source maps)
		errorSymbolicator := &testSymbolicator{
			shouldError:      true,
			errorMsg:         "source map not found: 404",
			returnFetchError: true,
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
		span.Attributes().PutStr(cfg.ExceptionTypeAttributeKey, "Error")
		span.Attributes().PutStr(cfg.ExceptionMessageAttributeKey, "test error")
		span.Attributes().PutStr(cfg.StackTraceAttributeKey,
			"Error: test error\n"+
				"    at f1 (missing.js:100:1)\n"+
				"    at f2 (missing.js:200:2)\n"+
				"    at f3 (missing.js:300:3)\n"+
				"    at f4 (missing.js:400:4)\n"+
				"    at f5 (missing.js:500:5)\n"+
				"    at f6 (missing.js:600:6)\n"+
				"    at f7 (missing.js:700:7)\n"+
				"    at f8 (missing.js:800:8)\n"+
				"    at f9 (missing.js:900:9)\n"+
				"    at f10 (missing.js:1000:10)")

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
		stackAttr, ok := span.Attributes().Get(cfg.StackTraceAttributeKey)
		assert.True(t, ok)
		assert.Contains(t, stackAttr.Str(), "failed to fetch source map")
	})

	t.Run("non-fetch errors are not cached - parse errors cause retry per frame", func(t *testing.T) {
		// Create a symbolicator that returns generic errors (simulating parse errors, not fetch errors)
		parseErrorSymbolicator := &testSymbolicator{
			shouldError:      true,
			errorMsg:         "failed to parse source map: invalid JSON",
			returnFetchError: false, // NOT a FetchError - should not be cached
		}

		testTel := componenttest.NewTelemetry()
		parseErrorTb, tbErr := metadata.NewTelemetryBuilder(testTel.NewTelemetrySettings())
		assert.NoError(t, tbErr)
		defer parseErrorTb.Shutdown()

		parseErrorAttributes := attribute.NewSet(
			attribute.String("processor_type", "sourcemap"),
		)

		parseErrorProcessor := newSymbolicatorProcessor(ctx, cfg, processorhelper.Settings{
			TelemetrySettings: component.TelemetrySettings{
				Logger: zaptest.NewLogger(t),
			},
		}, parseErrorSymbolicator, parseErrorTb, parseErrorAttributes)

		td := ptrace.NewTraces()
		rs := td.ResourceSpans().AppendEmpty()
		ils := rs.ScopeSpans().AppendEmpty()
		span := ils.Spans().AppendEmpty()

		// Create 3 frames all from the same URL that will return parse errors
		span.Attributes().PutEmpty(cfg.ColumnsAttributeKey).SetEmptySlice().FromRaw([]any{1, 2, 3})
		span.Attributes().PutEmpty(cfg.FunctionsAttributeKey).SetEmptySlice().FromRaw([]any{"f1", "f2", "f3"})
		span.Attributes().PutEmpty(cfg.LinesAttributeKey).SetEmptySlice().FromRaw([]any{100, 200, 300})
		span.Attributes().PutEmpty(cfg.UrlsAttributeKey).SetEmptySlice().FromRaw([]any{"app.js", "app.js", "app.js"})
		span.Attributes().PutStr(cfg.ExceptionTypeAttributeKey, "Error")
		span.Attributes().PutStr(cfg.ExceptionMessageAttributeKey, "test error")
		span.Attributes().PutStr(cfg.StackTraceAttributeKey,
			"Error: test error\n"+
				"    at f1 (app.js:100:1)\n"+
				"    at f2 (app.js:200:2)\n"+
				"    at f3 (app.js:300:3)")

		_, processErr := parseErrorProcessor.processTraces(ctx, td)
		assert.NoError(t, processErr) // Processing should succeed even if symbolication fails

		// Should call symbolicate 3 times - parse errors are NOT cached
		// This is correct because parse errors might be transient or fixable
		assert.Equal(t, 3, parseErrorSymbolicator.callCount,
			"Expected 3 symbolication calls: parse errors should not be cached")

		// Verify symbolication_failed attribute is set
		attr, ok := span.Attributes().Get(cfg.SymbolicatorFailureAttributeKey)
		assert.True(t, ok)
		assert.True(t, attr.Bool())

		// Verify the stacktrace indicates parse failure (not fetch failure)
		stackAttr, ok := span.Attributes().Get(cfg.StackTraceAttributeKey)
		assert.True(t, ok)
		assert.Contains(t, stackAttr.Str(), "failed to parse source map")
	})
}

func TestLanguageFiltering(t *testing.T) {
	tests := []struct {
		name             string
		allowedLanguages []string
		signalLanguage   string
		hasLanguageAttr  bool
		shouldProcess    bool
	}{
		{
			name:             "empty allowed languages processes all signals",
			allowedLanguages: []string{},
			signalLanguage:   "javascript",
			hasLanguageAttr:  true,
			shouldProcess:    true,
		},
		{
			name:             "matching language processes signal",
			allowedLanguages: []string{"javascript", "typescript"},
			signalLanguage:   "javascript",
			hasLanguageAttr:  true,
			shouldProcess:    true,
		},
		{
			name:             "non-matching language skips signal",
			allowedLanguages: []string{"javascript", "typescript"},
			signalLanguage:   "java",
			hasLanguageAttr:  true,
			shouldProcess:    false,
		},
		{
			name:             "missing language attribute skips signal when filtering enabled",
			allowedLanguages: []string{"javascript", "typescript"},
			signalLanguage:   "",
			hasLanguageAttr:  false,
			shouldProcess:    false,
		},
		{
			name:             "case insensitive matching",
			allowedLanguages: []string{"JavaScript", "TypeScript"},
			signalLanguage:   "JAVASCRIPT",
			hasLanguageAttr:  true,
			shouldProcess:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			cfg := &Config{
				StackTraceAttributeKey:          "exception.stacktrace",
				SymbolicatorFailureAttributeKey: "exception.symbolicator.failed",
				LanguageAttributeKey:            "telemetry.sdk.language",
				AllowedLanguages:                tt.allowedLanguages,
			}

			symbolicator := &testSymbolicator{
				callCount: 0,
			}

			tb, tbErr := metadata.NewTelemetryBuilder(component.TelemetrySettings{
				Logger:        zaptest.NewLogger(t),
				MeterProvider: noop.NewMeterProvider(),
			})
			assert.NoError(t, tbErr)
			defer tb.Shutdown()

			attributes := attribute.NewSet(
				attribute.String("processor_type", "sourcemap"),
			)

			processor := newSymbolicatorProcessor(ctx, cfg, processorhelper.Settings{
				TelemetrySettings: component.TelemetrySettings{
					Logger: zaptest.NewLogger(t),
				},
			}, symbolicator, tb, attributes)

			logs := plog.NewLogs()
			rl := logs.ResourceLogs().AppendEmpty()
			sl := rl.ScopeLogs().AppendEmpty()
			lr := sl.LogRecords().AppendEmpty()

			attrs := lr.Attributes()
			attrs.PutStr("exception.stacktrace", "Error at line 42")

			if tt.hasLanguageAttr {
				attrs.PutStr("telemetry.sdk.language", tt.signalLanguage)
			}

			result, err := processor.processLogs(ctx, logs)

			assert.NoError(t, err)
			assert.NotNil(t, result)

			processedAttrs := result.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0).Attributes()

			_, hasProcessorType := processedAttrs.Get("honeycomb.processor_type")

			if tt.shouldProcess {
				assert.True(t, hasProcessorType)
			} else {
				assert.False(t, hasProcessorType)
				assert.Equal(t, 0, symbolicator.callCount)
			}
		})
	}
}
