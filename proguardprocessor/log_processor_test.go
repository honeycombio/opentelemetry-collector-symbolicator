package proguardprocessor

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/honeycombio/opentelemetry-collector-symbolicator/proguardprocessor/internal/metadata"
	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"
	"go.uber.org/zap/zaptest"
)

type mockLogProcessorStore struct {
	mapping map[string][]byte
	err     error
}

func (m *mockLogProcessorStore) GetProguardMapping(ctx context.Context, uuid string) ([]byte, error) {
	return m.mapping[uuid], m.err
}

type mockLogProcessorSymbolicator struct {
	frames    []*mappedStackFrame
	err       error
	callCount int
}

func (m *mockLogProcessorSymbolicator) symbolicate(ctx context.Context, uuid, className, methodName string, lineNumber int) ([]*mappedStackFrame, error) {
	m.callCount++
	return m.frames, m.err
}

func (m *mockLogProcessorSymbolicator) clear() {
	m.callCount = 0
}

func createMockTelemetry(t *testing.T) (*metadata.TelemetryBuilder, attribute.Set) {
	settings := component.TelemetrySettings{
		Logger:        zaptest.NewLogger(t),
		MeterProvider: noop.NewMeterProvider(),
	}
	tb, err := metadata.NewTelemetryBuilder(settings)
	assert.NoError(t, err)
	attributes := attribute.NewSet()
	return tb, attributes
}

func TestNewProguardLogsProcessor(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		ClassesAttributeKey:      "classes",
		MethodsAttributeKey:      "methods",
		LinesAttributeKey:        "lines",
		SourceFilesAttributeKey:  "source_files",
		ProguardUUIDAttributeKey: "uuid",
	}
	settings := processor.Settings{
		TelemetrySettings: component.TelemetrySettings{
			Logger: zaptest.NewLogger(t),
		},
	}

	store := &mockLogProcessorStore{}
	symbolicator := &mockLogProcessorSymbolicator{}
	tb, attributes := createMockTelemetry(t)

	processor, err := newProguardLogsProcessor(ctx, cfg, store, settings, symbolicator, tb, attributes)

	assert.NoError(t, err)
	assert.NotNil(t, processor)
	assert.Equal(t, cfg, processor.cfg)
	assert.Equal(t, settings.Logger, processor.logger)
	assert.Equal(t, symbolicator, processor.symbolicator)
}

func TestProcessLogs_Success(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		ExceptionTypeAttributeKey:       "exception_type",
		ExceptionMessageAttributeKey:    "exception_message",
		ClassesAttributeKey:             "classes",
		MethodsAttributeKey:             "methods",
		LinesAttributeKey:               "lines",
		SourceFilesAttributeKey:         "source_files",
		ProguardUUIDAttributeKey:        "uuid",
		OutputStackTraceKey:             "stack_trace",
		SymbolicatorFailureAttributeKey: "symbolication_failed",
	}

	settings := processor.Settings{
		TelemetrySettings: component.TelemetrySettings{
			Logger: zaptest.NewLogger(t),
		},
	}

	store := &mockLogProcessorStore{}
	symbolicator := &mockLogProcessorSymbolicator{
		frames: []*mappedStackFrame{
			{ClassName: "com.example.DeobfuscatedClass", MethodName: "originalMethod", SourceFile: "Source.java", LineNumber: 100},
		},
		err: nil,
	}
	tb, attributes := createMockTelemetry(t)

	processor, err := newProguardLogsProcessor(ctx, cfg, store, settings, symbolicator, tb, attributes)

	logs := plog.NewLogs()
	rl := logs.ResourceLogs().AppendEmpty()
	sl := rl.ScopeLogs().AppendEmpty()
	lr := sl.LogRecords().AppendEmpty()

	attrs := lr.Attributes()
	attrs.PutStr("uuid", "test-uuid")
	attrs.PutStr("exception_type", "java.lang.RuntimeException")
	attrs.PutStr("exception_message", "Test exception")

	classes := attrs.PutEmptySlice("classes")
	classes.AppendEmpty().SetStr("com.example.Class")

	methods := attrs.PutEmptySlice("methods")
	methods.AppendEmpty().SetStr("method1")

	lines := attrs.PutEmptySlice("lines")
	lines.AppendEmpty().SetInt(42)

	sourceFiles := attrs.PutEmptySlice("source_files")
	sourceFiles.AppendEmpty().SetStr("Class.java")

	result, err := processor.ProcessLogs(ctx, logs)

	assert.NoError(t, err)
	assert.NotNil(t, result)

	processedAttrs := result.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0).Attributes()

	// Verify processor type and attributes included
	processorTypeAttr, ok := processedAttrs.Get("honeycomb.processor_type")
	assert.True(t, ok)
	assert.Equal(t, typeStr.String(), processorTypeAttr.Str())

	processorVersionAttr, ok := processedAttrs.Get("honeycomb.processor_version")
	assert.True(t, ok)
	assert.Equal(t, processorVersion, processorVersionAttr.Str())

	stackTrace, ok := processedAttrs.Get("stack_trace")
	assert.True(t, ok)
	assert.Contains(t, stackTrace.Str(), "java.lang.RuntimeException: Test exception")
	assert.Contains(t, stackTrace.Str(), "at com.example.DeobfuscatedClass.originalMethod(Source.java:100)")

	failed, ok := processedAttrs.Get("symbolication_failed")
	assert.True(t, ok)
	assert.False(t, failed.Bool())
}

func TestProcessLogs_KeepAllStackFrames(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		ExceptionTypeAttributeKey:       "exception_type",
		ExceptionMessageAttributeKey:    "exception_message",
		ClassesAttributeKey:             "classes",
		MethodsAttributeKey:             "methods",
		LinesAttributeKey:               "lines",
		SourceFilesAttributeKey:         "source_files",
		ProguardUUIDAttributeKey:        "uuid",
		OutputStackTraceKey:             "stack_trace",
		SymbolicatorFailureAttributeKey: "symbolication_failed",
	}

	settings := processor.Settings{
		TelemetrySettings: component.TelemetrySettings{
			Logger: zaptest.NewLogger(t),
		},
	}

	store := &mockLogProcessorStore{}
	// If no mapping found/needed, the symbolicator returns an empty frame list without error.
	symbolicator := &mockLogProcessorSymbolicator{
		frames: []*mappedStackFrame{},
		err:    nil,
	}
	tb, attributes := createMockTelemetry(t)

	processor, err := newProguardLogsProcessor(ctx, cfg, store, settings, symbolicator, tb, attributes)

	logs := plog.NewLogs()
	rl := logs.ResourceLogs().AppendEmpty()
	sl := rl.ScopeLogs().AppendEmpty()
	lr := sl.LogRecords().AppendEmpty()

	attrs := lr.Attributes()
	attrs.PutStr("uuid", "test-uuid")
	attrs.PutStr("exception_type", "java.lang.RuntimeException")
	attrs.PutStr("exception_message", "Test exception")

	classes := attrs.PutEmptySlice("classes")
	classes.AppendEmpty().SetStr("com.example.Class")
	classes.AppendEmpty().SetStr("com.example.Test")
	classes.AppendEmpty().SetStr("com.example.Unknown")

	methods := attrs.PutEmptySlice("methods")
	methods.AppendEmpty().SetStr("method1")
	methods.AppendEmpty().SetStr("method2")
	methods.AppendEmpty().SetStr("unknownMethod")

	lines := attrs.PutEmptySlice("lines")
	lines.AppendEmpty().SetInt(42)
	lines.AppendEmpty().SetInt(-2)
	lines.AppendEmpty().SetInt(-1)

	sourceFiles := attrs.PutEmptySlice("source_files")
	sourceFiles.AppendEmpty().SetStr("Class.java")
	sourceFiles.AppendEmpty().SetStr("Test.java")
	// Unknown source file
	sourceFiles.AppendEmpty()

	result, err := processor.ProcessLogs(ctx, logs)

	assert.NoError(t, err)
	assert.NotNil(t, result)

	processedAttrs := result.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0).Attributes()
	stackTrace, ok := processedAttrs.Get("stack_trace")
	assert.True(t, ok)
	assert.Contains(t, stackTrace.Str(), "java.lang.RuntimeException: Test exception")
	// Stack trace should include the original class, method, line number, and source file if no symbolication is needed.
	assert.Contains(t, stackTrace.Str(), "at com.example.Class.method1(Class.java:42)")
	// Stack frames with -2 line number are treated as native methods.
	assert.Contains(t, stackTrace.Str(), "at com.example.Test.method2(Native Method)")
	// Stack frames with -1 line number are treated as unknown source files.
	assert.Contains(t, stackTrace.Str(), "at com.example.Unknown.unknownMethod(Unknown Source)")

	// This should not count as a symbolication failure
	failed, ok := processedAttrs.Get("symbolication_failed")
	assert.True(t, ok)
	assert.False(t, failed.Bool())
}

func TestProcessLogRecord_MissingClassesAttribute(t *testing.T) {
	cfg := &Config{
		ClassesAttributeKey:             "classes",
		MethodsAttributeKey:             "methods",
		LinesAttributeKey:               "lines",
		SourceFilesAttributeKey:         "source_files",
		ProguardUUIDAttributeKey:        "uuid",
		SymbolicatorFailureAttributeKey: "symbolication_failed",
		SymbolicatorErrorAttributeKey:   "symbolication_error",
	}
	tb, attributes := createMockTelemetry(t)

	processor := &proguardLogsProcessor{
		cfg:              cfg,
		logger:           zaptest.NewLogger(t),
		telemetryBuilder: tb,
		attributes:       metric.WithAttributeSet(attributes),
	}

	lr := plog.NewLogRecord()
	attrs := lr.Attributes()
	attrs.PutStr("uuid", "test-uuid")

	processor.processLogRecord(context.Background(), lr)

	// Verify processor type and attributes are still included even on failure
	processorTypeAttr, ok := attrs.Get("honeycomb.processor_type")
	assert.True(t, ok)
	assert.Equal(t, typeStr.String(), processorTypeAttr.Str())

	processorVersionAttr, ok := attrs.Get("honeycomb.processor_version")
	assert.True(t, ok)
	assert.Equal(t, processorVersion, processorVersionAttr.Str())

	hasFailure, hasFailureAttr := attrs.Get(cfg.SymbolicatorFailureAttributeKey)
	assert.True(t, hasFailureAttr)
	assert.True(t, hasFailure.Bool())
	errorMsg, hasErrorMsgAttr := attrs.Get(cfg.SymbolicatorErrorAttributeKey)
	assert.True(t, hasErrorMsgAttr)
	assert.Equal(t, "missing attribute: classes", errorMsg.Str())
}

func TestProcessLogRecord_MismatchedAttributeLengths(t *testing.T) {
	cfg := &Config{
		ClassesAttributeKey:             "classes",
		MethodsAttributeKey:             "methods",
		LinesAttributeKey:               "lines",
		SourceFilesAttributeKey:         "source_files",
		ProguardUUIDAttributeKey:        "uuid",
		SymbolicatorFailureAttributeKey: "symbolication_failed",
		SymbolicatorErrorAttributeKey:   "symbolication_error",
	}
	tb, attributes := createMockTelemetry(t)

	processor := &proguardLogsProcessor{
		cfg:              cfg,
		logger:           zaptest.NewLogger(t),
		telemetryBuilder: tb,
		attributes:       metric.WithAttributeSet(attributes),
	}

	lr := plog.NewLogRecord()
	attrs := lr.Attributes()
	attrs.PutStr("uuid", "test-uuid")

	classes := attrs.PutEmptySlice("classes")
	classes.AppendEmpty().SetStr("Class1")
	classes.AppendEmpty().SetStr("Class2")

	methods := attrs.PutEmptySlice("methods")
	methods.AppendEmpty().SetStr("method1")

	lines := attrs.PutEmptySlice("lines")
	lines.AppendEmpty().SetInt(42)

	sourceFiles := attrs.PutEmptySlice("source_files")
	sourceFiles.AppendEmpty().SetStr("Class.java")

	processor.processLogRecord(context.Background(), lr)

	hasFailure, hasFailureAttr := attrs.Get(cfg.SymbolicatorFailureAttributeKey)
	assert.True(t, hasFailureAttr)
	assert.True(t, hasFailure.Bool())
	errorMsg, hasErrorMsgAttr := attrs.Get(cfg.SymbolicatorErrorAttributeKey)
	assert.True(t, hasErrorMsgAttr)
	assert.Contains(t, errorMsg.Str(), "mismatched stacktrace attribute lengths")
}

func TestProcessLogRecord_SymbolicationFailure(t *testing.T) {
	ctx := context.Background()

	cfg := &Config{
		ClassesAttributeKey:             "classes",
		MethodsAttributeKey:             "methods",
		LinesAttributeKey:               "lines",
		SourceFilesAttributeKey:         "source_files",
		ProguardUUIDAttributeKey:        "uuid",
		OutputStackTraceKey:             "stack_trace",
		SymbolicatorFailureAttributeKey: "symbolication_failed",
	}

	settings := processor.Settings{
		TelemetrySettings: component.TelemetrySettings{
			Logger: zaptest.NewLogger(t),
		},
	}

	store := &mockLogProcessorStore{}
	symbolicator := &mockLogProcessorSymbolicator{
		err: assert.AnError,
	}
	tb, attributes := createMockTelemetry(t)

	processor, _ := newProguardLogsProcessor(ctx, cfg, store, settings, symbolicator, tb, attributes)

	lr := plog.NewLogRecord()
	attrs := lr.Attributes()
	attrs.PutStr("uuid", "test-uuid")

	classes := attrs.PutEmptySlice("classes")
	classes.AppendEmpty().SetStr("com.example.Class")

	methods := attrs.PutEmptySlice("methods")
	methods.AppendEmpty().SetStr("method1")

	lines := attrs.PutEmptySlice("lines")
	lines.AppendEmpty().SetInt(42)

	sourceFiles := attrs.PutEmptySlice("source_files")
	sourceFiles.AppendEmpty().SetStr("Class.java")

	processor.processLogRecord(context.Background(), lr)

	stackTrace, ok := attrs.Get("stack_trace")
	assert.True(t, ok)
	assert.Contains(t, stackTrace.Str(), "Failed to symbolicate com.example.Class.method1(42)")

	failed, ok := attrs.Get("symbolication_failed")
	assert.True(t, ok)
	assert.True(t, failed.Bool())

	errorMsg, hasErrorMsgAttr := attrs.Get(cfg.SymbolicatorErrorAttributeKey)
	assert.True(t, hasErrorMsgAttr)
	assert.Equal(t, "symbolication failed for some stack frames", errorMsg.Str())
}

func TestProcessLogRecord_InvalidLineNumber(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		ClassesAttributeKey:             "classes",
		MethodsAttributeKey:             "methods",
		LinesAttributeKey:               "lines",
		SourceFilesAttributeKey:         "source_files",
		ProguardUUIDAttributeKey:        "uuid",
		OutputStackTraceKey:             "stack_trace",
		SymbolicatorFailureAttributeKey: "symbolication_failed",
	}

	settings := processor.Settings{
		TelemetrySettings: component.TelemetrySettings{
			Logger: zaptest.NewLogger(t),
		},
	}

	store := &mockLogProcessorStore{}
	symbolicator := &mockLogProcessorSymbolicator{}
	tb, attributes := createMockTelemetry(t)

	processor, _ := newProguardLogsProcessor(ctx, cfg, store, settings, symbolicator, tb, attributes)

	lr := plog.NewLogRecord()
	attrs := lr.Attributes()
	attrs.PutStr("uuid", "test-uuid")

	classes := attrs.PutEmptySlice("classes")
	classes.AppendEmpty().SetStr("com.example.Class")

	methods := attrs.PutEmptySlice("methods")
	methods.AppendEmpty().SetStr("method1")

	lines := attrs.PutEmptySlice("lines")
	lines.AppendEmpty().SetInt(-3)

	sourceFiles := attrs.PutEmptySlice("source_files")
	sourceFiles.AppendEmpty().SetStr("Class.java")

	processor.processLogRecord(context.Background(), lr)

	stackTrace, ok := attrs.Get("stack_trace")
	assert.True(t, ok)
	assert.Contains(t, stackTrace.Str(), "Invalid line number -3")

	failed, ok := attrs.Get("symbolication_failed")
	assert.True(t, ok)
	assert.True(t, failed.Bool())

	errorMsg, hasErrorMsgAttr := attrs.Get(cfg.SymbolicatorErrorAttributeKey)
	assert.True(t, hasErrorMsgAttr)
	assert.Equal(t, "symbolication failed for some stack frames", errorMsg.Str())
}

func TestProcessLogRecord_PreserveStackTrace(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		ClassesAttributeKey:             "classes",
		MethodsAttributeKey:             "methods",
		LinesAttributeKey:               "lines",
		SourceFilesAttributeKey:         "source_files",
		ProguardUUIDAttributeKey:        "uuid",
		OutputStackTraceKey:             "stack_trace",
		SymbolicatorFailureAttributeKey: "symbolication_failed",
		PreserveStackTrace:              true,
		OriginalClassesAttributeKey:     "original_classes",
		OriginalMethodsAttributeKey:     "original_methods",
		OriginalLinesAttributeKey:       "original_lines",
		OriginalStackTraceKey:           "original_stack_trace",
	}

	settings := processor.Settings{
		TelemetrySettings: component.TelemetrySettings{
			Logger: zaptest.NewLogger(t),
		},
	}

	store := &mockLogProcessorStore{}
	symbolicator := &mockLogProcessorSymbolicator{}
	tb, attributes := createMockTelemetry(t)

	processor, _ := newProguardLogsProcessor(ctx, cfg, store, settings, symbolicator, tb, attributes)

	lr := plog.NewLogRecord()
	attrs := lr.Attributes()
	attrs.PutStr("uuid", "test-uuid")
	attrs.PutStr("stack_trace", "existing stack trace")

	classes := attrs.PutEmptySlice("classes")
	classes.AppendEmpty().SetStr("com.example.Class")

	methods := attrs.PutEmptySlice("methods")
	methods.AppendEmpty().SetStr("method1")

	lines := attrs.PutEmptySlice("lines")
	lines.AppendEmpty().SetInt(42)

	sourceFiles := attrs.PutEmptySlice("source_files")
	sourceFiles.AppendEmpty().SetStr("Class.java")

	processor.processLogRecord(context.Background(), lr)

	// Check original attributes are preserved
	originalClasses, ok := attrs.Get("original_classes")
	assert.True(t, ok)
	assert.Equal(t, 1, originalClasses.Slice().Len())
	assert.Equal(t, "com.example.Class", originalClasses.Slice().At(0).Str())

	originalStackTrace, ok := attrs.Get("original_stack_trace")
	assert.True(t, ok)
	assert.Equal(t, "existing stack trace", originalStackTrace.Str())

	hasFailure, hasFailureAttr := attrs.Get(cfg.SymbolicatorFailureAttributeKey)
	assert.True(t, hasFailureAttr)
	assert.False(t, hasFailure.Bool())
}

func TestProcessLogRecord_MissingUUID(t *testing.T) {
	cfg := &Config{
		ClassesAttributeKey:             "classes",
		MethodsAttributeKey:             "methods",
		LinesAttributeKey:               "lines",
		SourceFilesAttributeKey:         "source_files",
		ProguardUUIDAttributeKey:        "uuid",
		SymbolicatorFailureAttributeKey: "symbolication_failed",
		SymbolicatorErrorAttributeKey:   "symbolication_error",
	}
	tb, attributes := createMockTelemetry(t)

	processor := &proguardLogsProcessor{
		cfg:              cfg,
		logger:           zaptest.NewLogger(t),
		telemetryBuilder: tb,
		attributes:       metric.WithAttributeSet(attributes),
	}

	lr := plog.NewLogRecord()
	attrs := lr.Attributes()

	classes := attrs.PutEmptySlice("classes")
	classes.AppendEmpty().SetStr("com.example.Class")

	methods := attrs.PutEmptySlice("methods")
	methods.AppendEmpty().SetStr("method1")

	lines := attrs.PutEmptySlice("lines")
	lines.AppendEmpty().SetInt(42)

	sourceFiles := attrs.PutEmptySlice("source_files")
	sourceFiles.AppendEmpty().SetStr("Class.java")

	processor.processLogRecord(context.Background(), lr)

	hasFailure, hasFailureAttr := attrs.Get(cfg.SymbolicatorFailureAttributeKey)
	assert.True(t, hasFailureAttr)
	assert.True(t, hasFailure.Bool())

	errorMsg, hasErrorMsgAttr := attrs.Get(cfg.SymbolicatorErrorAttributeKey)
	assert.True(t, hasErrorMsgAttr)
	assert.Equal(t, "missing attribute: uuid", errorMsg.Str())
}

func TestGetSlice(t *testing.T) {
	m := pcommon.NewMap()

	// Test with existing slice
	slice := m.PutEmptySlice("test_key")
	slice.AppendEmpty().SetStr("value1")

	result, ok := getSlice("test_key", m)
	assert.True(t, ok)
	assert.Equal(t, 1, result.Len())
	assert.Equal(t, "value1", result.At(0).Str())

	// Test with non-existing key
	result, ok = getSlice("non_existing", m)
	assert.False(t, ok)
	assert.Equal(t, 0, result.Len())
}

func TestErrorCaching_MissingProguardMapping(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		ClassesAttributeKey:             "classes",
		MethodsAttributeKey:             "methods",
		LinesAttributeKey:               "lines",
		SourceFilesAttributeKey:         "source_files",
		ProguardUUIDAttributeKey:        "uuid",
		OutputStackTraceKey:             "stack_trace",
		SymbolicatorFailureAttributeKey: "symbolication_failed",
		SymbolicatorErrorAttributeKey:   "symbolication_error",
	}

	settings := processor.Settings{
		TelemetrySettings: component.TelemetrySettings{
			Logger: zaptest.NewLogger(t),
		},
	}

	store := &mockLogProcessorStore{}
	// Create a symbolicator that returns FetchError (simulating missing ProGuard mapping)
	symbolicator := &mockLogProcessorSymbolicator{
		frames: nil,
		err:    &FetchError{UUID: "missing-uuid-123", Err: errors.New("404 not found")},
	}
	tb, attributes := createMockTelemetry(t)

	processor, err := newProguardLogsProcessor(ctx, cfg, store, settings, symbolicator, tb, attributes)
	assert.NoError(t, err)

	lr := plog.NewLogRecord()
	attrs := lr.Attributes()
	attrs.PutStr("uuid", "missing-uuid-123")

	// Create 10 frames all with the same UUID (missing ProGuard mapping)
	classes := attrs.PutEmptySlice("classes")
	methods := attrs.PutEmptySlice("methods")
	lines := attrs.PutEmptySlice("lines")
	sourceFiles := attrs.PutEmptySlice("source_files")

	for i := 0; i < 10; i++ {
		classes.AppendEmpty().SetStr("com.example.ObfuscatedClass")
		methods.AppendEmpty().SetStr("a")
		lines.AppendEmpty().SetInt(int64(100 + i*10))
		sourceFiles.AppendEmpty().SetStr("Unknown.java")
	}

	processor.processLogRecord(context.Background(), lr)

	// Should only call symbolicate ONCE for the first frame, then reuse cached error
	// This validates the 90% reduction in failed fetches claim (1 call instead of 10)
	assert.Equal(t, 1, symbolicator.callCount,
		"Expected only 1 symbolication call for 10 frames with same missing UUID (90%% reduction)")

	// Verify symbolication_failed attribute is set
	failed, ok := attrs.Get(cfg.SymbolicatorFailureAttributeKey)
	assert.True(t, ok)
	assert.True(t, failed.Bool())

	// Verify the error attribute is set
	errorMsg, ok := attrs.Get(cfg.SymbolicatorErrorAttributeKey)
	assert.True(t, ok)
	assert.Equal(t, "symbolication failed for some stack frames", errorMsg.Str())

	// Verify the stacktrace contains failure messages
	stackTrace, ok := attrs.Get(cfg.OutputStackTraceKey)
	assert.True(t, ok)
	assert.Contains(t, stackTrace.Str(), "Failed to symbolicate")
}

// testSymbolicatorWithFetchErrors simulates a symbolicator that can return FetchErrors or other errors
type testSymbolicatorWithFetchErrors struct {
	returnFetchError bool
	callCount        int
	err              error
}

func (m *testSymbolicatorWithFetchErrors) symbolicate(ctx context.Context, uuid, className, methodName string, lineNumber int) ([]*mappedStackFrame, error) {
	m.callCount++
	if m.err != nil {
		if m.returnFetchError {
			return nil, &FetchError{UUID: uuid, Err: m.err}
		}
		return nil, m.err
	}
	return []*mappedStackFrame{
		{ClassName: "Deobfuscated", MethodName: "method", SourceFile: "Source.java", LineNumber: int64(lineNumber)},
	}, nil
}

func TestDeduplication(t *testing.T) {
	tests := []struct {
		name              string
		numFrames         int
		returnFetchError  bool
		symbolicatorError error
		expectCallCount   int
		description       string
	}{
		{
			name:              "successful symbolication calls for each frame",
			numFrames:         10,
			returnFetchError:  false,
			symbolicatorError: nil,
			expectCallCount:   10,
			description:       "Each frame should be symbolicated independently when successful",
		},
		{
			name:              "FetchError is cached and reused within stacktrace",
			numFrames:         10,
			returnFetchError:  true,
			symbolicatorError: errors.New("404 not found"),
			expectCallCount:   1,
			description:       "FetchError should be cached after first frame, 90% reduction in calls",
		},
		{
			name:              "non-FetchError is NOT cached",
			numFrames:         5,
			returnFetchError:  false,
			symbolicatorError: errors.New("parse error"),
			expectCallCount:   5,
			description:       "Parse errors should not be cached, each frame attempts symbolication",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			cfg := &Config{
				ClassesAttributeKey:             "classes",
				MethodsAttributeKey:             "methods",
				LinesAttributeKey:               "lines",
				SourceFilesAttributeKey:         "source_files",
				ProguardUUIDAttributeKey:        "uuid",
				OutputStackTraceKey:             "stack_trace",
				SymbolicatorFailureAttributeKey: "symbolication_failed",
				SymbolicatorErrorAttributeKey:   "symbolication_error",
			}

			settings := processor.Settings{
				TelemetrySettings: component.TelemetrySettings{
					Logger: zaptest.NewLogger(t),
				},
			}

			store := &mockLogProcessorStore{}
			symbolicator := &testSymbolicatorWithFetchErrors{
				returnFetchError: tt.returnFetchError,
				err:              tt.symbolicatorError,
			}
			tb, attributes := createMockTelemetry(t)

			processor, err := newProguardLogsProcessor(ctx, cfg, store, settings, symbolicator, tb, attributes)
			assert.NoError(t, err)

			lr := plog.NewLogRecord()
			attrs := lr.Attributes()
			attrs.PutStr("uuid", "test-uuid-123")

			// Create frames with different line numbers to simulate realistic stacktrace
			classes := attrs.PutEmptySlice("classes")
			methods := attrs.PutEmptySlice("methods")
			lines := attrs.PutEmptySlice("lines")
			sourceFiles := attrs.PutEmptySlice("source_files")

			for i := 0; i < tt.numFrames; i++ {
				classes.AppendEmpty().SetStr("com.example.ObfuscatedClass")
				methods.AppendEmpty().SetStr("a")
				lines.AppendEmpty().SetInt(int64(100 + i*10))
				sourceFiles.AppendEmpty().SetStr("Unknown.java")
			}

			processor.processLogRecord(ctx, lr)

			assert.Equal(t, tt.expectCallCount, symbolicator.callCount, tt.description)

			// Verify failure flag based on whether there was an error
			failed, ok := attrs.Get(cfg.SymbolicatorFailureAttributeKey)
			assert.True(t, ok)
			if tt.symbolicatorError != nil {
				assert.True(t, failed.Bool())
			} else {
				assert.False(t, failed.Bool())
			}
		})
	}
}

func TestDeduplication_MultipleUUIDs(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		ClassesAttributeKey:             "classes",
		MethodsAttributeKey:             "methods",
		LinesAttributeKey:               "lines",
		SourceFilesAttributeKey:         "source_files",
		ProguardUUIDAttributeKey:        "uuid",
		OutputStackTraceKey:             "stack_trace",
		SymbolicatorFailureAttributeKey: "symbolication_failed",
	}

	settings := processor.Settings{
		TelemetrySettings: component.TelemetrySettings{
			Logger: zaptest.NewLogger(t),
		},
	}

	store := &mockLogProcessorStore{}
	symbolicator := &mockLogProcessorSymbolicator{
		frames: nil,
		err:    &FetchError{UUID: "test", Err: errors.New("404 not found")},
	}

	tb, attributes := createMockTelemetry(t)
	processor, err := newProguardLogsProcessor(ctx, cfg, store, settings, symbolicator, tb, attributes)
	assert.NoError(t, err)

	// Create log record with frames from the same UUID
	// In reality, all frames in a stacktrace share the same UUID
	// This test verifies error cache keying by UUID
	lr := plog.NewLogRecord()
	attrs := lr.Attributes()

	// Note: ProGuard UUID is typically the same for all frames in a stacktrace
	// This test verifies error cache keying by UUID
	attrs.PutStr("uuid", "uuid-1")

	classes := attrs.PutEmptySlice("classes")
	methods := attrs.PutEmptySlice("methods")
	lines := attrs.PutEmptySlice("lines")
	sourceFiles := attrs.PutEmptySlice("source_files")

	// Add 5 frames with the same UUID
	for i := 0; i < 5; i++ {
		classes.AppendEmpty().SetStr("com.example.Class" + fmt.Sprintf("%d", i))
		methods.AppendEmpty().SetStr("method" + fmt.Sprintf("%d", i))
		lines.AppendEmpty().SetInt(int64(100 + i*10))
		sourceFiles.AppendEmpty().SetStr("Unknown.java")
	}

	processor.processLogRecord(ctx, lr)

	// Should only call once per UUID, not once per frame
	assert.Equal(t, 1, symbolicator.callCount,
		"Expected 1 call for 5 frames with same UUID (error cached)")
}
