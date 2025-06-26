package dsymprocessor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
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
			attributes := log.Attributes()

			// if we have a stack trace, try symbolicating it
			if _, ok := attributes.Get(sp.cfg.StackTraceAttributeKey); ok {
				err := sp.processStackTraceAttributes(ctx, attributes)
				if err != nil {
					attributes.PutBool(sp.cfg.SymbolicatorFailureAttributeKey, true)
					attributes.PutStr("exception.symbolicator.error", err.Error())
					sp.logger.Debug("Error processing span", zap.Error(err))
				}

				continue
			}

			// no stack trace, let's check if there's a metrickit attribute
			if _, ok := attributes.Get(sp.cfg.MetricKitStackTraceAttributeKey); ok {
				err := sp.processMetricKitAttributes(ctx, attributes)
				if err != nil {
					attributes.PutBool(sp.cfg.SymbolicatorFailureAttributeKey, true)
					attributes.PutStr("exception.symbolicator.error", err.Error())
					sp.logger.Debug("Error processing span", zap.Error(err))
				}

				continue
			}

			// neither attribute exists, do nothing
			err := fmt.Errorf("%w: %s or %s", errMissingAttribute, sp.cfg.StackTraceAttributeKey, sp.cfg.MetricKitStackTraceAttributeKey)
			sp.logger.Debug("Error processing span", zap.Error(err))
		}
	}
}

func formatStackFrames(prefix, binaryName string, offset uint64, frames []*mappedDSYMStackFrame) string {
	lines := make([]string, len(frames))
	for i, loc := range frames {
		lines[i] = fmt.Sprintf("%s %s() (in %s) (%s:%d) + %d", prefix, loc.symbol, binaryName, loc.path, loc.line, offset)
	}

	return strings.Join(lines, "\n")
}

func (sp *symbolicatorProcessor) processStackTraceAttributes(ctx context.Context, attributes pcommon.Map) error {
	var ok bool
	var stackTraceValue pcommon.Value
	var binaryNameValue pcommon.Value
	var buildUUIDValue pcommon.Value

	if stackTraceValue, ok = attributes.Get(sp.cfg.StackTraceAttributeKey); !ok {
		// we should never get here (our caller checks this)
		return fmt.Errorf("Invalid state! Called proceStackTraceAttributes while missing %s attribute", sp.cfg.StackTraceAttributeKey)
	}
	rawStackTrace := stackTraceValue.Str()

	if buildUUIDValue, ok = attributes.Get(sp.cfg.BuildUUIDAttributeKey); !ok {
		return fmt.Errorf("%w: %s", errMissingAttribute, sp.cfg.BuildUUIDAttributeKey)
	}
	buildUUID := buildUUIDValue.Str()

	if binaryNameValue, ok = attributes.Get(sp.cfg.AppExecutableAttributeKey); !ok {
		return fmt.Errorf("%w: %s", errMissingAttribute, sp.cfg.AppExecutableAttributeKey)
	}
	binaryName := binaryNameValue.Str()

	lines := strings.Split(rawStackTrace, "\n")
	res := make([]string, len(lines))
	for idx, line := range lines {
		symbolicated, err := sp.symbolicateStackLine(ctx, line, binaryName, buildUUID)
		if err != nil {
			sp.logger.Debug("could not symbolicate line")
			res[idx] = line
			continue
		}
		res[idx] = symbolicated
	}

	if sp.cfg.PreserveStackTrace {
		attributes.PutStr(sp.cfg.OriginalStackTraceKey, rawStackTrace)
	}
	attributes.PutStr(sp.cfg.StackTraceAttributeKey, strings.Join(res, "\n"))

	return nil
}

// groups: line number, library name, hex address, uuid or binary name, offset
var stackLineRegex = regexp.MustCompile(`^([0-9]+)\s+([\w _\-\.]+[\w_\-\.])\s+(0x[\da-f]+)\s+([\w _\-\.]*) \+ (\d+)`)
var uuidRegex = regexp.MustCompile(`[0-9A-Z]{8}-[0-9A-Z]{4}-[0-9A-Z]{4}-[0-9A-Z]{4}-[0-9A-Z]{12}`)

func (sp *symbolicatorProcessor) symbolicateStackLine(ctx context.Context, line, binaryName, buildUUID string) (string, error) {
	matches := stackLineRegex.FindStringSubmatch(line)
	matchIdxes := stackLineRegex.FindStringSubmatchIndex(line)
	libName := matches[2]
	uuidOrBinary := matches[4]
	offsetInt, err := strconv.Atoi(matches[5])
	if err != nil {
		return "", err
	}
	offset := uint64(offsetInt)

	var uuid string
	var bin string
	if isUUID(uuidOrBinary) {
		uuid = uuidOrBinary
		bin = libName
	} else if uuidOrBinary == binaryName {
		uuid = buildUUID
		bin = binaryName
	} else {
		return line, nil
	}

	locations, err := sp.symbolicator.symbolicateFrame(ctx, uuid, bin, offset)

	if errors.Is(err, errFailedToFindDSYM) {
		return line, nil
	}
	if err != nil {
		return "", err
	}

	// keep everything up to the end of match group 3 (the binary/uuid)
	//   indexes are paired, so group 0 spans index 0 - index 1
	//   so index 7 is the end of group 3
	prefix := line[:matchIdxes[7]]

	return formatStackFrames(prefix, bin, offset, locations), nil
}

func isUUID(maybeUUID string) bool {
	return uuidRegex.MatchString(maybeUUID)
}

func formatMetricKitStackFrames(frame MetricKitCallStackFrame, frames []*mappedDSYMStackFrame) string {
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

func (sp *symbolicatorProcessor) processMetricKitAttributes(ctx context.Context, attributes pcommon.Map) error {
	var ok bool
	var metrickitStackTraceValue pcommon.Value

	if metrickitStackTraceValue, ok = attributes.Get(sp.cfg.MetricKitStackTraceAttributeKey); !ok {
		// we should never get here (our caller checks this)
		return fmt.Errorf("Invalid state! Called processMetricKitAttributes while missing %s attribute", sp.cfg.MetricKitStackTraceAttributeKey)
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

	// and we need to set exception.type and exception.message to make this a semantically valid exception
	sp.setMetricKitExceptionAttrs(ctx, attributes)

	return nil
}

func (sp *symbolicatorProcessor) setMetricKitExceptionAttrs(ctx context.Context, attributes pcommon.Map) {
	exceptionType := getFirstAvailableString(
		attributes,
		[]string{
			"metrickit.diagnostic.crash.exception.objc.type",
			"metrickit.diagnostic.crash.exception.mach_exception.name",
			"metrickit.diagnostic.crash.exception.signal.name",
		},
		"Unknown Error",
	)

	exceptionMsg := getFirstAvailableString(
		attributes,
		[]string{
			"metrickit.diagnostic.crash.exception.objc.message",
			"metrickit.diagnostic.crash.exception.mach_exception.description",
			"metrickit.diagnostic.crash.exception.signal.description",
			"metrickit.diagnostic.crash.exception.termination_reason",
		},
		"Unknown Error",
	)

	attributes.PutStr(sp.cfg.OutputMetricKitExceptionTypeAttributeKey, exceptionType)
	attributes.PutStr(sp.cfg.OutputMetricKitExceptionMessageAttributeKey, exceptionMsg)
}

func (sp *symbolicatorProcessor) symbolicateFrame(ctx context.Context, frame MetricKitCallStackFrame) (string, error) {
	locations, err := sp.symbolicator.symbolicateFrame(ctx, frame.BinaryUUID, frame.BinaryName, frame.OffsetIntoBinaryTextSegment)

	if errors.Is(err, errFailedToFindDSYM) {
		return fmt.Sprintf("%s(%s) +%d", frame.BinaryName, frame.BinaryUUID, frame.OffsetIntoBinaryTextSegment), nil
	}
	if err != nil {
		return "", err
	}

	return formatMetricKitStackFrames(frame, locations), nil
}

func getStackDepth(root MetricKitCallStackFrame) int {
	if root.SubFrames == nil || len(*root.SubFrames) == 0 {
		return 1
	}
	return 1 + getStackDepth((*root.SubFrames)[0])
}

func getFirstAvailableString(attributes pcommon.Map, keys []string, fallbackValue string) string {
	for _, key := range keys {
		value, ok := attributes.Get(key)
		if ok {
			return value.Str()
		}
	}
	return fallbackValue
}
