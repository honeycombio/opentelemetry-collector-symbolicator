package dsymprocessor

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/pdata/plog"
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

func (ts *testSymbolicator) symbolicateFrame(ctx context.Context, debugId, binaryName string, addr uint64) ([]*mappedDSYMStackFrame, error) {
	if debugId != "6A8CB813-45F6-3652-AD33-778FD1EAB196" {
		return nil, errFailedToFindDSYM
	}
	frame := mappedDSYMStackFrame{
		path:      "MyFile.swift",
		instrAddr: 1,
		lang:      "swift",
		line:      1,
		symAddr:   1,
		symbol:    "main",
	}
	return []*mappedDSYMStackFrame{&frame}, nil
}

func TestProcessStackTrace(t *testing.T) {
	ctx := context.Background()
	cfg := createDefaultConfig().(*Config)
	s := &testSymbolicator{}
	processor := newSymbolicatorProcessor(ctx, cfg, processor.Settings{
		TelemetrySettings: component.TelemetrySettings{
			Logger: zaptest.NewLogger(t),
		},
	}, s)

	stacktrace := `0   CoreFoundation                      0x00000001835df228 7821F73C-378B-3A10-BE90-EF526B7DBA93 + 1155624
1   libobjc.A.dylib                     0x0000000180a79abc objc_exception_throw + 88
2   CoreFoundation                      0x00000001835e15fc 7821F73C-378B-3A10-BE90-EF526B7DBA93 + 1164796
3   Chateaux Bufeaux                    0x00000001025a0758 Chateaux Bufeaux + 231256
4   Chateaux Bufeaux                    0x00000001025a0834 Chateaux Bufeaux + 231476
5   Chateaux Bufeaux                    0x000000010259f2ac Chateaux Bufeaux + 225964
6   Chateaux Bufeaux                    0x0000000102577fd1 Chateaux Bufeaux + 65489
7   libswift_Concurrency.dylib          0x000000018f0a9241 DCB9E73A-92BA-3782-BC6D-3E1906622689 + 414273`

	s.clear()

	for _, preserveStack := range []bool{true, false} {
		t.Run(fmt.Sprintf("processAttributes with preserveStack = %s", strconv.FormatBool(preserveStack)), func(t *testing.T) {
			cfg.PreserveStackTrace = preserveStack

			logs := plog.NewLogs()
			resourceLog := logs.ResourceLogs().AppendEmpty()
			scopeLog := resourceLog.ScopeLogs().AppendEmpty()
			
			log := scopeLog.LogRecords().AppendEmpty()
			log.SetEventName("error")
			log.Attributes().PutEmpty(cfg.StackTraceAttributeKey).SetStr(stacktrace)
			log.Attributes().PutEmpty(cfg.BuildUUIDAttributeKey).SetStr("6A8CB813-45F6-3652-AD33-778FD1EAB196")
			log.Attributes().PutEmpty(cfg.AppExecutableAttributeKey).SetStr("Chateaux Bufeaux")

			err := processor.processStackTraceAttributes(ctx, log.Attributes())
			assert.NoError(t, err)

			symbolicated, found := log.Attributes().Get(cfg.StackTraceAttributeKey)
			assert.True(t, found)

			expected := `0   CoreFoundation                      0x00000001835df228 7821F73C-378B-3A10-BE90-EF526B7DBA93 + 1155624
1   libobjc.A.dylib                     0x0000000180a79abc objc_exception_throw + 88
2   CoreFoundation                      0x00000001835e15fc 7821F73C-378B-3A10-BE90-EF526B7DBA93 + 1164796
3   Chateaux Bufeaux                    0x00000001025a0758 main() (in Chateaux Bufeaux) (MyFile.swift:1) + 231256
4   Chateaux Bufeaux                    0x00000001025a0834 main() (in Chateaux Bufeaux) (MyFile.swift:1) + 231476
5   Chateaux Bufeaux                    0x000000010259f2ac main() (in Chateaux Bufeaux) (MyFile.swift:1) + 225964
6   Chateaux Bufeaux                    0x0000000102577fd1 main() (in Chateaux Bufeaux) (MyFile.swift:1) + 65489
7   libswift_Concurrency.dylib          0x000000018f0a9241 DCB9E73A-92BA-3782-BC6D-3E1906622689 + 414273`

			assert.Equal(t, expected, symbolicated.Str())

			// no failures
			hasError, found := log.Attributes().Get(cfg.SymbolicatorFailureAttributeKey)
			assert.True(t, found)
			assert.False(t, hasError.Bool())

			// original json is preserved based on key
			originalStackTrace, found := log.Attributes().Get(cfg.OriginalStackTraceKey)
			if preserveStack {
				assert.True(t, found)
				assert.Equal(t, stacktrace, originalStackTrace.Str())
			} else {
				assert.False(t, found)
			}
		})
	}
}

func TestProcessMetricKit(t *testing.T) {
	ctx := context.Background()
	cfg := createDefaultConfig().(*Config)
	s := &testSymbolicator{}
	processor := newSymbolicatorProcessor(ctx, cfg, processor.Settings{
		TelemetrySettings: component.TelemetrySettings{
			Logger: zaptest.NewLogger(t),
		},
	}, s)

	jsonstr := `{
		"callStacks": [
			{
				"threadAttributed": true,
				"callStackRootFrames": [
					{
						"binaryUUID": "6527276E-A3D1-30FB-BA68-ACA33324D618",
						"offsetIntoBinaryTextSegment": 933484,
						"sampleCount": 1,
						"subFrames": [
							{
								"binaryUUID": "6527276E-A3D1-30FB-BA68-ACA33324D618",
								"offsetIntoBinaryTextSegment": 933200,
								"sampleCount": 1,
								"subFrames": [
									{
										"binaryUUID": "6A8CB813-45F6-3652-AD33-778FD1EAB196",
										"offsetIntoBinaryTextSegment": 100436,
										"sampleCount": 1,
										"subFrames": [
											{
												"binaryUUID": "189FE480-5D5B-3B89-9289-58BC88624420",
												"offsetIntoBinaryTextSegment": 68312,
												"sampleCount": 1,
												"binaryName": "dyld",
												"address": 7540112088
											}
										],
										"binaryName": "Chateaux Bufeaux",
										"address": 4365699156
									}
								],
								"binaryName": "SwiftUI",
								"address": 6968069456
							}
						],
						"binaryName": "SwiftUI",
						"address": 6968069740
					}
				]
			}
		]
	}`

	s.clear()

	for _, preserveStack := range []bool{true, false} {
		t.Run(fmt.Sprintf("processAttributes with preserveStack = %s", strconv.FormatBool(preserveStack)), func(t *testing.T) {
			cfg.PreserveStackTrace = preserveStack

			logs := plog.NewLogs()
			resourceLog := logs.ResourceLogs().AppendEmpty()
			scopeLog := resourceLog.ScopeLogs().AppendEmpty()
			
			log := scopeLog.LogRecords().AppendEmpty()
			log.SetEventName("metrickit.diagnostic.crash")
			log.Attributes().PutEmpty(cfg.MetricKitStackTraceAttributeKey).SetStr(jsonstr)

			err := processor.processMetricKitAttributes(ctx, log.Attributes())
			assert.NoError(t, err)

			symbolicated, found := log.Attributes().Get(cfg.OutputMetricKitStackTraceAttributeKey)
			assert.True(t, found)

			expected := `dyld(189FE480-5D5B-3B89-9289-58BC88624420) +68312
    Chateaux Bufeaux			0x18854 main() (MyFile.swift:1) + 1
    SwiftUI(6527276E-A3D1-30FB-BA68-ACA33324D618) +933200
    SwiftUI(6527276E-A3D1-30FB-BA68-ACA33324D618) +933484`

			assert.Equal(t, expected, symbolicated.Str())

			// no failures
			hasError, found := log.Attributes().Get(cfg.SymbolicatorFailureAttributeKey)
			assert.True(t, found)
			assert.False(t, hasError.Bool())

			// original json is preserved based on key
			metrickitJson, found := log.Attributes().Get(cfg.MetricKitStackTraceAttributeKey)
			if preserveStack {
				assert.True(t, found)
				assert.Equal(t, jsonstr, metrickitJson.Str())
			} else {
				assert.False(t, found)
			}

			exceptionType, found := log.Attributes().Get(cfg.OutputMetricKitExceptionTypeAttributeKey)
			assert.True(t, found)
			assert.Equal(t, "Unknown Error", exceptionType.Str())

			exceptionMessage, found := log.Attributes().Get(cfg.OutputMetricKitExceptionMessageAttributeKey)
			assert.True(t, found)
			assert.Equal(t, "Unknown Error", exceptionMessage.Str())
		})
	}
}

func TestMetricKitExceptionAttrs(t *testing.T) {
	ctx := context.Background()
	cfg := createDefaultConfig().(*Config)
	s := &testSymbolicator{}
	processor := newSymbolicatorProcessor(ctx, cfg, processor.Settings{
		TelemetrySettings: component.TelemetrySettings{
			Logger: zaptest.NewLogger(t),
		},
	}, s)

	jsonstr := `{ "callStacks": [] }`

	logs := plog.NewLogs()
	resourceLog := logs.ResourceLogs().AppendEmpty()
	scopeLog := resourceLog.ScopeLogs().AppendEmpty()
	
	log := scopeLog.LogRecords().AppendEmpty()
	log.SetEventName("metrickit.diagnostic.crash")
	log.Attributes().PutEmpty(cfg.MetricKitStackTraceAttributeKey).SetStr(jsonstr)
	log.Attributes().PutEmpty("metrickit.diagnostic.crash.exception.mach_exception.name").SetStr("exception type")
	log.Attributes().PutEmpty("metrickit.diagnostic.crash.exception.mach_exception.description").SetStr("message")

	err := processor.processMetricKitAttributes(ctx, log.Attributes())
	assert.NoError(t, err)

	exceptionType, found := log.Attributes().Get(cfg.OutputMetricKitExceptionTypeAttributeKey)
	assert.True(t, found)
	assert.Equal(t, "exception type", exceptionType.Str())

	exceptionMessage, found := log.Attributes().Get(cfg.OutputMetricKitExceptionMessageAttributeKey)
	assert.True(t, found)
	assert.Equal(t, "message", exceptionMessage.Str())

	// add the objc ones, they should take precedence
	log.Attributes().PutEmpty("metrickit.diagnostic.crash.exception.objc.type").SetStr("objc exception type")
	log.Attributes().PutEmpty("metrickit.diagnostic.crash.exception.objc.message").SetStr("objc message")

	err = processor.processMetricKitAttributes(ctx, log.Attributes())
	assert.NoError(t, err)

	exceptionType, found= log.Attributes().Get(cfg.OutputMetricKitExceptionTypeAttributeKey)
	assert.True(t, found)
	assert.Equal(t, "objc exception type", exceptionType.Str())

	exceptionMessage, found = log.Attributes().Get(cfg.OutputMetricKitExceptionMessageAttributeKey)
	assert.True(t, found)
	assert.Equal(t, "objc message", exceptionMessage.Str())
}

func TestProcessFailure_WrongKey(t *testing.T) {
	ctx := context.Background()
	cfg := createDefaultConfig().(*Config)
	s := &testSymbolicator{}
	processor := newSymbolicatorProcessor(ctx, cfg, processor.Settings{
		TelemetrySettings: component.TelemetrySettings{
			Logger: zaptest.NewLogger(t),
		},
	}, s)

	jsonstr := `{
		"callStacks": [
			{
				"threadAttributed": true,
				"callStackRootFrames": [
					{
						"binaryUUID": "6527276E",
						"offsetIntoBinaryTextSegment": 933484,
						"sampleCount": 1,
						"subFrames": [
							{
								"binaryUUID": "6527276E",
								"offsetIntoBinaryTextSegment": 933200,
								"sampleCount": 1,
								"subFrames": [
									{
										"binaryUUID": "6A8CB813",
										"offsetIntoBinaryTextSegment": 100436,
										"sampleCount": 1,
										"subFrames": [
											{
												"binaryUUID": "189FE480",
												"offsetIntoBinaryTextSegment": 68312,
												"sampleCount": 1,
												"binaryName": "dyld",
												"address": 7540112088
											}
										],
										"binaryName": "Chateaux Bufeaux",
										"address": 4365699156
									}
								],
								"binaryName": "SwiftUI",
								"address": 6968069456
							}
						],
						"binaryName": "SwiftUI",
						"address": 6968069740
					}
				]
			}
		]
	}`

	s.clear()

	logs := plog.NewLogs()
	resourceLog := logs.ResourceLogs().AppendEmpty()
	scopeLog := resourceLog.ScopeLogs().AppendEmpty()
	
	log := scopeLog.LogRecords().AppendEmpty()
	log.SetEventName("metrickit.diagnostic.crash")
	log.Attributes().PutEmpty("incorrect.attribute.key").SetStr(jsonstr)

	err := processor.processMetricKitAttributes(ctx, log.Attributes())
	assert.Error(t, err)

	_, found := log.Attributes().Get(cfg.OutputMetricKitStackTraceAttributeKey)
	assert.False(t, found)

	hasError, found := log.Attributes().Get(cfg.SymbolicatorFailureAttributeKey)
	assert.True(t, found)
	assert.True(t, hasError.Bool())
}

func TestProcessFailure_InvalidJson(t *testing.T) {
	ctx := context.Background()
	cfg := createDefaultConfig().(*Config)
	s := &testSymbolicator{}
	processor := newSymbolicatorProcessor(ctx, cfg, processor.Settings{
		TelemetrySettings: component.TelemetrySettings{
			Logger: zaptest.NewLogger(t),
		},
	}, s)

	jsonstr := `not a json stacktrace`

	s.clear()

	logs := plog.NewLogs()
	resourceLog := logs.ResourceLogs().AppendEmpty()
	scopeLog := resourceLog.ScopeLogs().AppendEmpty()
	
	log := scopeLog.LogRecords().AppendEmpty()
	log.SetEventName("metrickit.diagnostic.crash")
	log.Attributes().PutEmpty("incorrect.attribute.key").SetStr(jsonstr)
	

	err := processor.processMetricKitAttributes(ctx, log.Attributes())
	assert.Error(t, err)

	_, found := log.Attributes().Get(cfg.OutputMetricKitStackTraceAttributeKey)
	assert.False(t, found)

	hasError, found := log.Attributes().Get(cfg.SymbolicatorFailureAttributeKey)
	assert.True(t, found)
	assert.True(t, hasError.Bool())
}
