package dsymprocessor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/processor"
	"go.uber.org/zap"
)

var (
	errMissingAttribute = errors.New("missing attribute")
)

// symbolicator interface is used to symbolicate stack traces.
type symbolicator interface {
	symbolicateFrame(ctx context.Context, debugId, binaryName string, addr uint64) ([]*mappedDSYMStackFrame, error)
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
func (sp *symbolicatorProcessor) processLogs(ctx context.Context, logs plog.Logs) (plog.Logs, error) {
	sp.logger.Info("Processing logs")

	for i := 0; i < logs.ResourceLogs().Len(); i++ {
		rl := logs.ResourceLogs().At(i)
		sp.processResourceSpans(ctx, rl)
	}
	return logs, nil
}

// processResourceSpans takes resource spans and processes the attributes
// found on the spans.
func (sp *symbolicatorProcessor) processResourceSpans(ctx context.Context, rl plog.ResourceLogs) {
	for i := 0; i < rl.ScopeLogs().Len(); i++ {
		sl := rl.ScopeLogs().At(i)

		for j := 0; j < sl.LogRecords().Len(); j++ {
			log := sl.LogRecords().At(j)

			err := sp.processAttributes(ctx, log.Attributes())

			if err != nil {
				sp.logger.Debug("Error processing span", zap.Error(err))
			}
		}
	}
}

func formatStackFrames(frame MetricKitCallStackFrame, frames []*mappedDSYMStackFrame) string {
	lines := make([]string, len(frames))
	for i, loc := range frames {
		lines[i] = fmt.Sprintf("%s\t\t\t0x%X %s() (%s:%d) + %d", frame.BinaryName, frame.OffsetIntoBinaryTextSegment, loc.symbol, loc.path, loc.line, loc.symAddr)
	}

	return strings.Join(lines, "\n")
}

type MetricKitCrashReport struct {
	CallStacks []MetricKitCallStack
}
type MetricKitCallStack struct {
	ThreadAttributed    bool
	CallStackRootFrames []MetricKitCallStackFrame
}
type MetricKitCallStackFrame struct {
	BinaryUUID                  string
	OffsetIntoBinaryTextSegment uint64
	SubFrames                   *[]MetricKitCallStackFrame
	BinaryName                  string
}

func (sp *symbolicatorProcessor) processAttributes(ctx context.Context, attributes pcommon.Map) error {
	// set this true at the beginning. If we succeed, we'll hit the "set to false" call at the end of this function
	attributes.PutBool(sp.cfg.SymbolicatorFailureAttributeKey, true)

	var ok bool
	var metrickitStackTraceValue pcommon.Value

	if metrickitStackTraceValue, ok = attributes.Get(sp.cfg.MetricKitStackTraceAttributeKey); !ok {
		return fmt.Errorf("%w: %s", errMissingAttribute, sp.cfg.MetricKitStackTraceAttributeKey)
	}
	metrickitStackTrace := metrickitStackTraceValue.Str()

	var report MetricKitCrashReport

	err := json.Unmarshal([]byte(metrickitStackTrace), &report)
	if err != nil {
		return err
	}

	// deepest nested frames are at the top of the stack
	// gotta unwind the nesting in reverse
	stacks := make([]string, len(report.CallStacks))

	for idx, callStack := range report.CallStacks {
		capacity := getStackDepth(callStack.CallStackRootFrames[0])
		symbolicatedStack := make([]string, capacity)
		frame := callStack.CallStackRootFrames[0]
		for i := capacity - 1; i >= 0; i-- {
			line, err := sp.symbolicateFrame(ctx, frame)
			if err != nil {
				return err
			}
			symbolicatedStack[i] = line
			frames := frame.SubFrames
			if frames == nil {
				continue
			}
			frame = (*frames)[0]
		}

		stacks[idx] = strings.Join(symbolicatedStack, "\n    ")
	}

	attributes.PutStr(sp.cfg.OutputMetricKitStackTraceAttributeKey, strings.Join(stacks, "\n\n\n"))
	if !sp.cfg.PreserveStackTrace {
		attributes.Remove(sp.cfg.MetricKitStackTraceAttributeKey)
	}

	// everything was a success, we can overwrite the `true` we put in at the beginning
	attributes.PutBool(sp.cfg.SymbolicatorFailureAttributeKey, false)
	return nil
}

func (sp *symbolicatorProcessor) symbolicateFrame(ctx context.Context, frame MetricKitCallStackFrame) (string, error) {
	locations, err := sp.symbolicator.symbolicateFrame(ctx, frame.BinaryUUID, frame.BinaryName, frame.OffsetIntoBinaryTextSegment)

	if errors.Is(err, errFailedToFindDSYM) {
		return fmt.Sprintf("%s(%s) +%d", frame.BinaryName, frame.BinaryUUID, frame.OffsetIntoBinaryTextSegment), nil
	}
	if err != nil {
		return "", err
	}

	return formatStackFrames(frame, locations), nil
}

func getStackDepth(root MetricKitCallStackFrame) int {
	if root.SubFrames == nil || len(*root.SubFrames) == 0 {
		return 1
	}
	return 1 + getStackDepth((*root.SubFrames)[0])
}
