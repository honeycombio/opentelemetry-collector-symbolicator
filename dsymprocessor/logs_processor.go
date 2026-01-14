package dsymprocessor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/honeycombio/opentelemetry-collector-symbolicator/dsymprocessor/internal/metadata"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.uber.org/zap"
)

var (
	errMissingAttribute     = errors.New("missing attribute")
	errPartialSymbolication = errors.New("symbolication failed for some stack frames")
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

	telemetryBuilder *metadata.TelemetryBuilder
	attributes       metric.MeasurementOption
}

// newSymbolicatorProcessor creates a new symbolicatorProcessor.
func newSymbolicatorProcessor(_ context.Context, cfg *Config, set processor.Settings, symbolicator symbolicator, tb *metadata.TelemetryBuilder, attributes attribute.Set) *symbolicatorProcessor {
	return &symbolicatorProcessor{
		cfg:              cfg,
		logger:           set.Logger,
		symbolicator:     symbolicator,
		telemetryBuilder: tb,
		attributes:       metric.WithAttributeSet(attributes),
	}
}

// processTraces processes the received traces. It is the function configured
// in the processorhelper.NewTraces call in factory.go
func (sp *symbolicatorProcessor) processLogs(ctx context.Context, logs plog.Logs) (plog.Logs, error) {
	sp.logger.Debug("Processing logs")

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
			resourceAttrs := rl.Resource().Attributes()

			// Check language filtering if configured
			if len(sp.cfg.AllowedLanguages) > 0 {
				// Get language attribute from log attributes or resource attributes
				languageValue, ok := attributes.Get(sp.cfg.LanguageAttributeKey)
				if !ok {
					languageValue, ok = resourceAttrs.Get(sp.cfg.LanguageAttributeKey)
				}

				// If language attribute exists, check if it matches allowed languages
				if ok {
					language := languageValue.Str()
					if !isLanguageAllowed(language, sp.cfg.AllowedLanguages) {
						continue
					}
				} else { // Language attribute not found, skip processing
					continue
				}
			}

			// if we have a stack trace, try symbolicating it
			if _, ok := attributes.Get(sp.cfg.StackTraceAttributeKey); ok {
				// Check if this is a MetricKit diagnostic via eventName
				eventName := log.EventName()
				if strings.HasPrefix(eventName, "metrickit.diagnostic.") {
					// MetricKit JSON format
					sp.processMetricKitAttributes(ctx, attributes)
				} else {
					// Regular text format
					sp.processStackTraceAttributes(ctx, attributes, resourceAttrs)
				}
				continue
			}

			// no stack trace, let's check if there's a metrickit attribute (for backwards compatibility)
			if _, ok := attributes.Get(sp.cfg.MetricKitStackTraceAttributeKey); ok {
				sp.processMetricKitAttributes(ctx, attributes)
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
		lines[i] = fmt.Sprintf("%s %s (in %s) (%s:%d) + %d", prefix, loc.symbol, binaryName, loc.path, loc.line, offset)
	}

	return strings.Join(lines, "\n")
}

func (sp *symbolicatorProcessor) processStackTraceAttributes(ctx context.Context, attributes pcommon.Map, resourceAttributes pcommon.Map) {
	// Start timing symbolication only when we actually perform it
	// End timing deferred to after processing is done
	startTime := time.Now()
	defer func() {
		sp.telemetryBuilder.ProcessorSymbolicationDuration.Record(ctx, time.Since(startTime).Seconds(), sp.attributes)
	}()

	// Add processor type and version as attributes
	attributes.PutStr("honeycomb.processor_type", typeStr.String())
	attributes.PutStr("honeycomb.processor_version", processorVersion)

	err := sp.processStackTraceAttributesThrows(ctx, attributes, resourceAttributes)
	if err != nil {
		attributes.PutBool(sp.cfg.SymbolicatorFailureAttributeKey, true)
		attributes.PutStr("exception.symbolicator.error", err.Error())
		sp.logger.Debug("Error processing span", zap.Error(err))
	} else {
		attributes.PutBool(sp.cfg.SymbolicatorFailureAttributeKey, false)
	}
}

func (sp *symbolicatorProcessor) processStackTraceAttributesThrows(ctx context.Context, attributes pcommon.Map, resourceAttributes pcommon.Map) error {
	var ok bool
	var stackTraceValue pcommon.Value
	var binaryNameValue pcommon.Value
	var buildUUIDValue pcommon.Value

	if stackTraceValue, ok = attributes.Get(sp.cfg.StackTraceAttributeKey); !ok {
		// we should never get here (our caller checks this)
		return fmt.Errorf("Invalid state! Called proceStackTraceAttributes while missing %s attribute", sp.cfg.StackTraceAttributeKey)
	}
	rawStackTrace := stackTraceValue.Str()

	if buildUUIDValue, ok = resourceAttributes.Get(sp.cfg.BuildUUIDAttributeKey); !ok {
		return fmt.Errorf("%w: %s", errMissingAttribute, sp.cfg.BuildUUIDAttributeKey)
	}
	buildUUID := buildUUIDValue.Str()

	if binaryNameValue, ok = resourceAttributes.Get(sp.cfg.AppExecutableAttributeKey); !ok {
		return fmt.Errorf("%w: %s", errMissingAttribute, sp.cfg.AppExecutableAttributeKey)
	}
	binaryName := binaryNameValue.Str()

	lines := strings.Split(rawStackTrace, "\n")
	res := make([]string, len(lines))
	symbolicationFailed := false

	// Cache FetchErrors to avoid redundant fetches for missing resources.
	fetchErrorCache := make(map[string]error)

	for idx, line := range lines {
		symbolicated, err := sp.symbolicateStackLine(ctx, line, binaryName, buildUUID, fetchErrorCache)
		if err != nil {
			sp.logger.Debug("could not symbolicate line")
			res[idx] = line
			symbolicationFailed = true
			continue
		}
		res[idx] = symbolicated
	}

	if sp.cfg.PreserveStackTrace {
		attributes.PutStr(sp.cfg.OriginalStackTraceAttributeKey, rawStackTrace)
	}
	attributes.PutStr(sp.cfg.StackTraceAttributeKey, strings.Join(res, "\n"))

	if symbolicationFailed {
		return errPartialSymbolication
	} else {
		return nil
	}
}

// groups: stack index, library name, hex address, uuid or binary name, offset
var stackLineRegex = regexp.MustCompile(`^([0-9]+)\s+([\w _\-\.]+[\w_\-\.])\s+(0x[\da-f]+)\s+([\w _\-\.]*) \+ (\d+)`)
var uuidRegex = regexp.MustCompile(`[0-9A-Z]{8}-[0-9A-Z]{4}-[0-9A-Z]{4}-[0-9A-Z]{4}-[0-9A-Z]{12}`)

func (sp *symbolicatorProcessor) symbolicateStackLine(ctx context.Context, line, binaryName, buildUUID string, fetchErrorCache map[string]error) (string, error) {
	if !stackLineRegex.MatchString(line) {
		// stacktrace line not formated the way we expect, skip it
		return line, nil
	}
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

	// Check if we have a cached fetch error for this UUID
	if cachedError, exists := fetchErrorCache[uuid]; exists {
		return "", cachedError
	}

	locations, err := sp.symbolicator.symbolicateFrame(ctx, uuid, bin, offset)
	sp.telemetryBuilder.ProcessorTotalProcessedFrames.Add(ctx, 1, sp.attributes)

	// Only cache FetchErrors (404, timeout, etc.) - not parse errors
	if err != nil {
		var fetchErr *FetchError
		if errors.As(err, &fetchErr) {
			fetchErrorCache[uuid] = err
		}
	}

	if errors.Is(err, errFailedToFindDSYM) {
		return line, nil
	}
	if err != nil {
		sp.telemetryBuilder.ProcessorTotalFailedFrames.Add(ctx, 1, sp.attributes)
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
	var offset uint64 = 0
	if frame.OffsetIntoBinaryTextSegment != nil {
		offset = *frame.OffsetIntoBinaryTextSegment
	} else if frame.OffsetAddress != nil {
		offset = *frame.OffsetAddress
	}

	lines := make([]string, len(frames))
	for i, loc := range frames {
		lines[i] = fmt.Sprintf("%s\t\t\t0x%X %s (%s:%d) + %d", frame.BinaryName, offset, loc.symbol, loc.path, loc.line, loc.symAddr)
	}

	return strings.Join(lines, "\n")
}

type MetricKitCrashReport struct {
	CallStacks []MetricKitCallStack `json:"callStacks"`
}

type MetricKitCallStack struct {
	ThreadAttributed bool `json:"threadAttributed"`

	// the original Apple format for MetricKit stack traces
	CallStackRootFrames *[]MetricKitCallStackFrame `json:"callStackRootFrames"`

	// the flattened OpenTelemetry format for MetricKit stack traces
	CallStackFrames *[]MetricKitCallStackFrame `json:"callStackFrames"`
}

type MetricKitCallStackFrame struct {
	BinaryName string `json:"binaryName"`
	BinaryUUID string `json:"binaryUUID"`

	// the original Apple format
	OffsetIntoBinaryTextSegment *uint64                    `json:"offsetIntoBinaryTextSegment"`
	SubFrames                   *[]MetricKitCallStackFrame `json:"subFrames"`

	// the simplified OpenTelemetry format
	OffsetAddress *uint64 `json:"offsetAddress"`
}

func (sp *symbolicatorProcessor) processMetricKitAttributes(ctx context.Context, attributes pcommon.Map) {
	// Start timing symbolication only when we actually perform it
	// End timing deferred to after processing is done
	startTime := time.Now()
	defer func() {
		sp.telemetryBuilder.ProcessorSymbolicationDuration.Record(ctx, time.Since(startTime).Seconds(), sp.attributes)
	}()

	// Add processor type and version as attributes
	attributes.PutStr("honeycomb.processor_type", typeStr.String())
	attributes.PutStr("honeycomb.processor_version", processorVersion)

	err := sp.processMetricKitAttributesThrows(ctx, attributes)
	if err != nil {
		attributes.PutBool(sp.cfg.SymbolicatorFailureAttributeKey, true)
		attributes.PutStr("exception.symbolicator.error", err.Error())
		sp.logger.Debug("Error processing span", zap.Error(err))
	} else {
		attributes.PutBool(sp.cfg.SymbolicatorFailureAttributeKey, false)
	}
}

func (sp *symbolicatorProcessor) processMetricKitAttributesThrows(ctx context.Context, attributes pcommon.Map) error {
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

	stacks := make([]string, 0, len(report.CallStacks))

	// Cache FetchErrors to avoid redundant fetches for missing resources.
	fetchErrorCache := make(map[string]error)

	for _, callStack := range report.CallStacks {
		symbolicatedStack := make([]string, 0, 512)

		// Try the old Apple format.
		if callStack.CallStackRootFrames != nil && len(*callStack.CallStackRootFrames) > 0 {
			frame := &(*callStack.CallStackRootFrames)[0]
			for frame != nil {
				line, err := sp.symbolicateFrame(ctx, *frame, fetchErrorCache)
				if err != nil {
					return err
				}

				symbolicatedStack = append(symbolicatedStack, line)

				frames := frame.SubFrames
				frame = nil
				if frames != nil && len(*frames) > 0 {
					frame = &(*frames)[0]
				}
			}
		}

		// Try the new OTel format.
		if callStack.CallStackFrames != nil {
			for _, frame := range *callStack.CallStackFrames {
				line, err := sp.symbolicateFrame(ctx, frame, fetchErrorCache)
				if err != nil {
					return err
				}
				symbolicatedStack = append(symbolicatedStack, line)
			}
		}

		stacks = append(stacks, strings.Join(symbolicatedStack, "\n    "))
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

func (sp *symbolicatorProcessor) symbolicateFrame(ctx context.Context, frame MetricKitCallStackFrame, fetchErrorCache map[string]error) (string, error) {
	// Check if we have a cached fetch error for this UUID
	if cachedError, exists := fetchErrorCache[frame.BinaryUUID]; exists {
		return "", cachedError
	}

	var offset uint64 = 0
	if frame.OffsetAddress != nil {
		offset = *frame.OffsetAddress
	}
	if frame.OffsetIntoBinaryTextSegment != nil {
		offset = *frame.OffsetIntoBinaryTextSegment
	}

	locations, err := sp.symbolicator.symbolicateFrame(ctx, frame.BinaryUUID, frame.BinaryName, offset)
	sp.telemetryBuilder.ProcessorTotalProcessedFrames.Add(ctx, 1, sp.attributes)

	// Only cache FetchErrors (404, timeout, etc.) - not parse errors
	if err != nil {
		var fetchErr *FetchError
		if errors.As(err, &fetchErr) {
			fetchErrorCache[frame.BinaryUUID] = err
		}
	}

	if errors.Is(err, errFailedToFindDSYM) {
		return fmt.Sprintf("%s(%s) +%d", frame.BinaryName, frame.BinaryUUID, offset), nil
	}
	if err != nil {
		sp.telemetryBuilder.ProcessorTotalFailedFrames.Add(ctx, 1, sp.attributes)
		return "", err
	}

	return formatMetricKitStackFrames(frame, locations), nil
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

// isLanguageAllowed checks if the given language matches any of the allowed languages.
// Comparison is case insensitive.
func isLanguageAllowed(language string, allowedLanguages []string) bool {
	language = strings.ToLower(language)
	for _, allowed := range allowedLanguages {
		if strings.ToLower(allowed) == language {
			return true
		}
	}
	return false
}
