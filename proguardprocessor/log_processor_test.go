package proguardprocessor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/processor"
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

func TestNewProguardLogsProcessor(t *testing.T) {
	ctx := context.Background()
	cfg := &Config{
		ClassesAttributeKey:      "classes",
		MethodsAttributeKey:      "methods",
		LinesAttributeKey:        "lines",
		ProguardUUIDAttributeKey: "uuid",
	}
	settings := processor.Settings{
		TelemetrySettings: component.TelemetrySettings{
			Logger: zaptest.NewLogger(t),
		},
	}

	store := &mockLogProcessorStore{}
	symbolicator := &mockLogProcessorSymbolicator{}

	processor, err := newProguardLogsProcessor(ctx, cfg, store, settings, symbolicator)

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

	processor, err := newProguardLogsProcessor(ctx, cfg, store, settings, symbolicator)

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

	result, err := processor.ProcessLogs(ctx, logs)

	assert.NoError(t, err)
	assert.NotNil(t, result)

	processedAttrs := result.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0).Attributes()
	stackTrace, ok := processedAttrs.Get("stack_trace")
	assert.True(t, ok)
	assert.Contains(t, stackTrace.Str(), "java.lang.RuntimeException: Test exception")
	assert.Contains(t, stackTrace.Str(), "at com.example.DeobfuscatedClass.originalMethod(Source.java:100)")

	failed, ok := processedAttrs.Get("symbolication_failed")
	assert.True(t, ok)
	assert.False(t, failed.Bool())
}

func TestProcessLogRecord_MissingClassesAttribute(t *testing.T) {
	cfg := &Config{
		ClassesAttributeKey:      "classes",
		MethodsAttributeKey:      "methods",
		LinesAttributeKey:        "lines",
		ProguardUUIDAttributeKey: "uuid",
		SymbolicatorFailureAttributeKey: "symbolication_failed",
		SymbolicatorErrorAttributeKey: "symbolication_error",
	}

	processor := &proguardLogsProcessor{
		cfg:    cfg,
		logger: zaptest.NewLogger(t),
	}

	lr := plog.NewLogRecord()
	attrs := lr.Attributes()
	attrs.PutStr("uuid", "test-uuid")

	processor.processLogRecord(context.Background(), lr)

	hasFailure, hasFailureAttr := attrs.Get(cfg.SymbolicatorFailureAttributeKey)
	assert.True(t, hasFailureAttr)
	assert.True(t, hasFailure.Bool())
	errorMsg, hasErrorMsgAttr := attrs.Get(cfg.SymbolicatorErrorAttributeKey)
	assert.True(t, hasErrorMsgAttr)
	assert.Equal(t, "missing attribute: classes", errorMsg.Str())
}

func TestProcessLogRecord_MismatchedAttributeLengths(t *testing.T) {
	cfg := &Config{
		ClassesAttributeKey:      "classes",
		MethodsAttributeKey:      "methods",
		LinesAttributeKey:        "lines",
		ProguardUUIDAttributeKey: "uuid",
		SymbolicatorFailureAttributeKey: "symbolication_failed",
		SymbolicatorErrorAttributeKey: "symbolication_error",
	}

	processor := &proguardLogsProcessor{
		cfg:    cfg,
		logger: zaptest.NewLogger(t),
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

	processor, _ := newProguardLogsProcessor(ctx, cfg, store, settings, symbolicator)

	lr := plog.NewLogRecord()
	attrs := lr.Attributes()
	attrs.PutStr("uuid", "test-uuid")

	classes := attrs.PutEmptySlice("classes")
	classes.AppendEmpty().SetStr("com.example.Class")

	methods := attrs.PutEmptySlice("methods")
	methods.AppendEmpty().SetStr("method1")

	lines := attrs.PutEmptySlice("lines")
	lines.AppendEmpty().SetInt(42)

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

	processor, _ := newProguardLogsProcessor(ctx, cfg, store, settings, symbolicator)

	lr := plog.NewLogRecord()
	attrs := lr.Attributes()
	attrs.PutStr("uuid", "test-uuid")

	classes := attrs.PutEmptySlice("classes")
	classes.AppendEmpty().SetStr("com.example.Class")

	methods := attrs.PutEmptySlice("methods")
	methods.AppendEmpty().SetStr("method1")

	lines := attrs.PutEmptySlice("lines")
	lines.AppendEmpty().SetInt(-1)

	processor.processLogRecord(context.Background(), lr)

	stackTrace, ok := attrs.Get("stack_trace")
	assert.True(t, ok)
	assert.Contains(t, stackTrace.Str(), "Invalid line number -1")

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

	processor, _ := newProguardLogsProcessor(ctx, cfg, store, settings, symbolicator)

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
		ClassesAttributeKey:      "classes",
		MethodsAttributeKey:      "methods",
		LinesAttributeKey:        "lines",
		ProguardUUIDAttributeKey: "uuid",
		SymbolicatorFailureAttributeKey: "symbolication_failed",
		SymbolicatorErrorAttributeKey:   "symbolication_error",
	}

	processor := &proguardLogsProcessor{
		cfg:    cfg,
		logger: zaptest.NewLogger(t),
	}

	lr := plog.NewLogRecord()
	attrs := lr.Attributes()

	classes := attrs.PutEmptySlice("classes")
	classes.AppendEmpty().SetStr("com.example.Class")

	methods := attrs.PutEmptySlice("methods")
	methods.AppendEmpty().SetStr("method1")

	lines := attrs.PutEmptySlice("lines")
	lines.AppendEmpty().SetInt(42)

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
