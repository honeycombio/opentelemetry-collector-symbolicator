package symbolicatorprocessor

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/ptrace"
	"go.opentelemetry.io/collector/processor"
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

func (ts *testSymbolicator) symbolicate(ctx context.Context, line, column int64, function, url string) (string, error) {
	ts.SymbolicatedLines = append(ts.SymbolicatedLines, symbolicatedLine{
		Line:     line,
		Column:   column,
		Function: function,
		URL:      url,
	})
	return fmt.Sprintf("symbolicated %d:%d %s %s", line, column, function, url), nil
}

func TestProcess(t *testing.T) {
	ctx := context.Background()
	cfg := createDefaultConfig().(*Config)
	s := &testSymbolicator{}
	processor := newSymbolicatorProcessor(ctx, cfg, processor.Settings{
		TelemetrySettings: component.TelemetrySettings{
			Logger: zaptest.NewLogger(t),
		},
	}, s)

	tts := []struct {
		Name                    string
		ApplyAttributes         func(attrs ptrace.Span)
		AssertSymbolicatorCalls func(s *testSymbolicator)
		AssertOutput            func(td ptrace.Traces)
	}{
		{
			Name: "attributes provided",
			ApplyAttributes: func(span ptrace.Span) {
				span.Attributes().PutEmpty(cfg.ColumnsAttributeKey).SetEmptySlice().AppendEmpty().SetInt(42)
				span.Attributes().PutEmpty(cfg.LinesAttributeKey).SetEmptySlice().AppendEmpty().SetInt(42)
				span.Attributes().PutEmpty(cfg.FunctionsAttributeKey).SetEmptySlice().AppendEmpty().SetStr("function")
				span.Attributes().PutEmpty(cfg.UrlsAttributeKey).SetEmptySlice().AppendEmpty().SetStr("url")
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
				attr, ok := span.Attributes().Get(cfg.OutputStackTraceKey)
				assert.True(t, ok)
				assert.Equal(t, "symbolicated 42:42 function url", attr.Str())
			},
		},
		// Add unit tests to include attributes preserving stack trace
		// Add unit test testing the preserveStackTrace option
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
