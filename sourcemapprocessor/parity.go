package sourcemapprocessor

import (
	"time"

	"go.opentelemetry.io/collector/pdata/pcommon"
)

// NOTE: This file contains parity checking logic for comparing TraceKit (structured) vs
// Collector-side parsing routes. This is for internal testing purposes and will be removed
// in the future once parity is validated.

// parityStatus represents the status of a parity check between two parsing routes
type parityStatus string

const (
	// All fields of the parsed frame are identical between routes.
	parityStatusConsistent parityStatus = "consistent"
	// Resulting field has values that are missing, different, or otherwise inconsistent between routes.
	parityStatusDifferent parityStatus = "different"
	// TraceKit failed to parse but Sourcemap Processor succeeded.
	parityStatusTracekitFailed parityStatus = "tracekit-failed"
	// Processor parser failed to parse but TraceKit succeeded.
	parityStatusProcessorParserFailed parityStatus = "processor-parser-failed"
	// Stackframe failed to parse in both routes.
	parityStatusParsingFailed parityStatus = "all-parsing-failed"
)

// frameComparisonType represents the result of comparing a stack frame between routes
type frameComparisonType string

const (
	// Stack Frame is different between both libraries.
	frameComparisonDifferent frameComparisonType = "different"
	// Stack Frame is consistent between both libraries.
	frameComparisonConsistent frameComparisonType = "consistent"
)

// addParityCheckAttributes adds parity check results as attributes to the current span/log
// Compares TraceKit (structured attributes) vs Collector-side parsed frames directly
func addParityCheckAttributes(
	attributes pcommon.Map,
	tracekitLines, tracekitColumns, tracekitFunctions, tracekitUrls pcommon.Slice,
	parsedStackTrace *stackTrace,
	duration time.Duration,
) {
	// Copy TraceKit slices directly
	tracekitLines.CopyTo(attributes.PutEmptySlice("tracekit.lines"))
	tracekitColumns.CopyTo(attributes.PutEmptySlice("tracekit.columns"))
	tracekitFunctions.CopyTo(attributes.PutEmptySlice("tracekit.functions"))
	tracekitUrls.CopyTo(attributes.PutEmptySlice("tracekit.urls"))

	// Extract processor-parsed frames into slices
	processorLines := attributes.PutEmptySlice("processorParser.lines")
	processorColumns := attributes.PutEmptySlice("processorParser.columns")
	processorFunctions := attributes.PutEmptySlice("processorParser.functions")
	processorUrls := attributes.PutEmptySlice("processorParser.urls")

	if parsedStackTrace != nil {
		for _, frame := range parsedStackTrace.stackFrames {
			processorUrls.AppendEmpty().SetStr(frame.url)
			processorFunctions.AppendEmpty().SetStr(frame.funcName)
			if frame.line != nil {
				processorLines.AppendEmpty().SetInt(int64(*frame.line))
			} else {
				processorLines.AppendEmpty().SetInt(-1)
			}
			if frame.column != nil {
				processorColumns.AppendEmpty().SetInt(int64(*frame.column))
			} else {
				processorColumns.AppendEmpty().SetInt(-1)
			}
		}
	}

	// Validate TraceKit data (all slices must have same length)
	tracekitValid := tracekitLines.Len() == tracekitColumns.Len() &&
		tracekitLines.Len() == tracekitFunctions.Len() &&
		tracekitLines.Len() == tracekitUrls.Len()

	processorValid := parsedStackTrace != nil

	status := parityStatusConsistent
	totalMismatches := 0
	comparisonTypes := attributes.PutEmptySlice("parity.stackframe.comparison")

	// Determine status based on both routes' validity
	if !tracekitValid && !processorValid {
		status = parityStatusParsingFailed
	} else if !tracekitValid && processorValid {
		status = parityStatusTracekitFailed
	} else if tracekitValid && !processorValid {
		status = parityStatusProcessorParserFailed
	} else if tracekitColumns.Len() != processorColumns.Len() {
		status = parityStatusDifferent // There's missing data between the 2 libraries
	} else {
		// Check the contents of both, should have same array size at this point
		for i := 0; i < processorColumns.Len(); i++ {
			if processorColumns.At(i).Int() != tracekitColumns.At(i).Int() ||
				processorLines.At(i).Int() != tracekitLines.At(i).Int() ||
				processorFunctions.At(i).Str() != tracekitFunctions.At(i).Str() ||
				processorUrls.At(i).Str() != tracekitUrls.At(i).Str() {
				comparisonTypes.AppendEmpty().SetStr(string(frameComparisonDifferent))
				status = parityStatusDifferent // we only need one frame to be off for the parsing to be different
				totalMismatches += 1
			} else {
				comparisonTypes.AppendEmpty().SetStr(string(frameComparisonConsistent))
			}
		}
	}

	
	// Set summary attributes
	attributes.PutStr("parity.status", string(status))
	attributes.PutInt("parity.totalMismatches", int64(totalMismatches))
	attributes.PutDouble("parity.duration", duration.Seconds())
}
