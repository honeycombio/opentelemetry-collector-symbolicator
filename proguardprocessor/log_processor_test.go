package proguardprocessor

import (
	"context"
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
	frames []*mappedStackFrame
	err    error
}

func (m *mockLogProcessorSymbolicator) symbolicate(ctx context.Context, uuid, className, methodName string, lineNumber int) ([]*mappedStackFrame, error) {
	return m.frames, m.err
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
		StackTraceAttributeKey:          "stack_trace",
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
		StackTraceAttributeKey:          "stack_trace",
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
		StackTraceAttributeKey:          "stack_trace",
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
	// Now with fallback parsing, the error message mentions both missing structured attributes and stack trace
	assert.Contains(t, errorMsg.Str(), "missing structured stack trace attributes")
	assert.Contains(t, errorMsg.Str(), "stack_trace attribute is missing")
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
		StackTraceAttributeKey:          "stack_trace",
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
		StackTraceAttributeKey:          "stack_trace",
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
		StackTraceAttributeKey:          "stack_trace",
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

func TestProcessLogRecord_FallbackToRawStackTraceParsing(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		ClassesAttributeKey:             "classes",
		MethodsAttributeKey:             "methods",
		LinesAttributeKey:               "lines",
		SourceFilesAttributeKey:         "source_files",
		ExceptionTypeAttributeKey:       "exception_type",
		ExceptionMessageAttributeKey:    "exception_message",
		ProguardUUIDAttributeKey:        "uuid",
		StackTraceAttributeKey:          "stack_trace",
		SymbolicatorFailureAttributeKey: "symbolication_failed",
	}

	settings := processor.Settings{
		TelemetrySettings: component.TelemetrySettings{
			Logger: zaptest.NewLogger(t),
		},
	}

	store := &mockLogProcessorStore{}
	// Return empty frames (no mapping needed) so we can verify the parsed values remain
	symbolicator := &mockLogProcessorSymbolicator{
		frames: []*mappedStackFrame{},
		err:    nil,
	}
	tb, attributes := createMockTelemetry(t)

	processor, _ := newProguardLogsProcessor(ctx, cfg, store, settings, symbolicator, tb, attributes)

	lr := plog.NewLogRecord()
	attrs := lr.Attributes()
	attrs.PutStr("uuid", "test-uuid")

	// Only provide raw stack trace, no structured attributes
	rawStackTrace := `java.lang.RuntimeException: Test exception
	at com.example.ObfuscatedClass.obfuscatedMethod(SourceFile:42)
	at com.example.AnotherClass.anotherMethod(Native Method)
	at com.example.ThirdClass.thirdMethod(Unknown Source)`
	attrs.PutStr("stack_trace", rawStackTrace)

	processor.processLogRecord(context.Background(), lr)

	// Verify that parsing succeeded and structured attributes were populated
	classes, ok := attrs.Get("classes")
	assert.True(t, ok)
	assert.Equal(t, 3, classes.Slice().Len())
	assert.Equal(t, "com.example.ObfuscatedClass", classes.Slice().At(0).Str())
	assert.Equal(t, "com.example.AnotherClass", classes.Slice().At(1).Str())
	assert.Equal(t, "com.example.ThirdClass", classes.Slice().At(2).Str())

	methods, ok := attrs.Get("methods")
	assert.True(t, ok)
	assert.Equal(t, 3, methods.Slice().Len())
	assert.Equal(t, "obfuscatedMethod", methods.Slice().At(0).Str())
	assert.Equal(t, "anotherMethod", methods.Slice().At(1).Str())
	assert.Equal(t, "thirdMethod", methods.Slice().At(2).Str())

	lines, ok := attrs.Get("lines")
	assert.True(t, ok)
	assert.Equal(t, 3, lines.Slice().Len())
	assert.Equal(t, int64(42), lines.Slice().At(0).Int())
	assert.Equal(t, int64(-2), lines.Slice().At(1).Int()) // Native Method
	assert.Equal(t, int64(-1), lines.Slice().At(2).Int()) // Unknown Source

	sourceFiles, ok := attrs.Get("source_files")
	assert.True(t, ok)
	assert.Equal(t, 3, sourceFiles.Slice().Len())
	assert.Equal(t, "SourceFile", sourceFiles.Slice().At(0).Str())
	assert.Equal(t, "Native Method", sourceFiles.Slice().At(1).Str())
	assert.Equal(t, "Unknown Source", sourceFiles.Slice().At(2).Str())

	// Verify symbolication succeeded
	failed, ok := attrs.Get("symbolication_failed")
	assert.True(t, ok)
	assert.False(t, failed.Bool())

	// Verify output stack trace was generated with original (unparsed) values
	stackTrace, ok := attrs.Get("stack_trace")
	assert.True(t, ok)
	assert.Contains(t, stackTrace.Str(), "java.lang.RuntimeException: Test exception")
	assert.Contains(t, stackTrace.Str(), "at com.example.ObfuscatedClass.obfuscatedMethod(SourceFile:42)")
	assert.Contains(t, stackTrace.Str(), "at com.example.AnotherClass.anotherMethod(Native Method)")
	assert.Contains(t, stackTrace.Str(), "at com.example.ThirdClass.thirdMethod(Unknown Source)")
}

func TestProcessLogRecord_MissingBothStructuredAndRawStackTrace(t *testing.T) {
	cfg := &Config{
		ClassesAttributeKey:             "classes",
		MethodsAttributeKey:             "methods",
		LinesAttributeKey:               "lines",
		SourceFilesAttributeKey:         "source_files",
		StackTraceAttributeKey:          "stack_trace",
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

	// No structured attributes and no raw stack trace provided
	processor.processLogRecord(context.Background(), lr)

	// Verify failure
	hasFailure, hasFailureAttr := attrs.Get(cfg.SymbolicatorFailureAttributeKey)
	assert.True(t, hasFailureAttr)
	assert.True(t, hasFailure.Bool())

	errorMsg, hasErrorMsgAttr := attrs.Get(cfg.SymbolicatorErrorAttributeKey)
	assert.True(t, hasErrorMsgAttr)
	assert.Contains(t, errorMsg.Str(), "missing structured stack trace attributes")
	assert.Contains(t, errorMsg.Str(), "stack_trace attribute is missing")
}

func TestProcessLogRecord_InvalidRawStackTraceFormat(t *testing.T) {
	cfg := &Config{
		ClassesAttributeKey:             "classes",
		MethodsAttributeKey:             "methods",
		LinesAttributeKey:               "lines",
		SourceFilesAttributeKey:         "source_files",
		StackTraceAttributeKey:          "stack_trace",
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

	// Provide invalid stack trace format
	attrs.PutStr("stack_trace", "This is not a valid stack trace format")

	processor.processLogRecord(context.Background(), lr)

	// Verify failure
	hasFailure, hasFailureAttr := attrs.Get(cfg.SymbolicatorFailureAttributeKey)
	assert.True(t, hasFailureAttr)
	assert.True(t, hasFailure.Bool())

	errorMsg, hasErrorMsgAttr := attrs.Get(cfg.SymbolicatorErrorAttributeKey)
	assert.True(t, hasErrorMsgAttr)
	assert.Contains(t, errorMsg.Str(), "failed to parse raw stack trace from stack_trace")
}
