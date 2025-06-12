package dsymprocessor

import (
	"context"
	"fmt"
	"strconv"
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

func TestProcess(t *testing.T) {
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

			td := ptrace.NewTraces()
			rs := td.ResourceSpans().AppendEmpty()
			ils := rs.ScopeSpans().AppendEmpty()

			span := ils.Spans().AppendEmpty()
			span.SetName("first-batch-first-span")
			span.SetTraceID([16]byte{1, 2, 3, 4})
			span.Attributes().PutEmpty(cfg.MetricKitStackTraceAttributeKey).SetStr(jsonstr)

			err := processor.processAttributes(ctx, span.Attributes())
			assert.NoError(t, err)

			symbolicated, found := span.Attributes().Get(cfg.OutputMetricKitStackTraceAttributeKey)
			assert.True(t, found)

			expected := `dyld(189FE480-5D5B-3B89-9289-58BC88624420) +68312
    Chateaux Bufeaux			0x18854 main() (MyFile.swift:1) + 1
    SwiftUI(6527276E-A3D1-30FB-BA68-ACA33324D618) +933200
    SwiftUI(6527276E-A3D1-30FB-BA68-ACA33324D618) +933484`

			assert.Equal(t, expected, symbolicated.Str())

			// no failures
			hasError, found := span.Attributes().Get(cfg.SymbolicatorFailureAttributeKey)
			assert.True(t, found)
			assert.False(t, hasError.Bool())

			// original json is preserved based on key
			metrickitJson, found := span.Attributes().Get(cfg.MetricKitStackTraceAttributeKey)
			if preserveStack {
				assert.True(t, found)
				assert.Equal(t, jsonstr, metrickitJson.Str())
			} else {
				assert.False(t, found)
			}
		})
	}
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

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	ils := rs.ScopeSpans().AppendEmpty()

	span := ils.Spans().AppendEmpty()
	span.SetName("first-batch-first-span")
	span.SetTraceID([16]byte{1, 2, 3, 4})
	span.Attributes().PutEmpty("incorrect.attribute.key").SetStr(jsonstr)

	err := processor.processAttributes(ctx, span.Attributes())
	assert.Error(t, err)

	_, found := span.Attributes().Get(cfg.OutputMetricKitStackTraceAttributeKey)
	assert.False(t, found)

	hasError, found := span.Attributes().Get(cfg.SymbolicatorFailureAttributeKey)
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

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	ils := rs.ScopeSpans().AppendEmpty()

	span := ils.Spans().AppendEmpty()
	span.SetName("first-batch-first-span")
	span.SetTraceID([16]byte{1, 2, 3, 4})
	span.Attributes().PutEmpty("incorrect.attribute.key").SetStr(jsonstr)

	err := processor.processAttributes(ctx, span.Attributes())
	assert.Error(t, err)

	_, found := span.Attributes().Get(cfg.OutputMetricKitStackTraceAttributeKey)
	assert.False(t, found)

	hasError, found := span.Attributes().Get(cfg.SymbolicatorFailureAttributeKey)
	assert.True(t, found)
	assert.True(t, hasError.Bool())
}
