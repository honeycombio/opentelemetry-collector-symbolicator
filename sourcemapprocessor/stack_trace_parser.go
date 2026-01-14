// This code was originally adapted from TraceKit, a JavaScript error tracing and
// stack trace parsing library.
// TraceKit is MIT-licensed and available at: https://github.com/csnover/TraceKit

package sourcemapprocessor

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	// unknownFunction is used when the function name cannot be determined or is missing.
	unknownFunction = "?"
)

// Compiled regular expressions for parsing stack traces from various browsers.
var (
	// React Native "address at" format (iOS and Android)
	// Format: "at funcName (address at path/to/file.bundle:line:column)"
	reactNativeRE = regexp.MustCompile(`(?i)^\s*at (.*?) ?\(address at (.+?)(?::(\d+))?(?::(\d+))?\)\s*$`)
	// Chrome/V8 stack trace format
	chromeRE = regexp.MustCompile(`(?i)^\s*at (.*?) ?\(((?:file|https?|blob|chrome-extension|native|eval|webpack|<anonymous>|\/).*?)(?::(\d+))?(?::(\d+))?\)?\s*$`)
	// Gecko/Firefox stack trace format
	geckoRE = regexp.MustCompile(`(?i)^\s*(.*?)(?:\((.*?)\))?(?:^|@)((?:file|https?|blob|chrome|webpack|resource|\[native).*?|[^@]*bundle)(?::(\d+))?(?::(\d+))?\s*$`)
	// Windows JavaScript (WinJS) stack trace format
	winJSRE = regexp.MustCompile(`(?i)^\s*at (?:((?:\[object object\])?.+) )?\(?((?:file|ms-appx|https?|webpack|blob):.*?):(\d+)(?::(\d+))?\)?\s*$`)
	// Gecko eval format
	geckoEvalRE = regexp.MustCompile(`(?i)(\S+) line (\d+)(?: > eval line \d+)* > eval`)
	// Chrome eval format (note: TraceKit doesn't have /i flag for this one)
	chromeEvalRE = regexp.MustCompile(`\((\S*)(?::(\d+))(?::(\d+))\)`)

	// Opera 11+ stacktrace property format (without column)
	opera11StacktraceRE = regexp.MustCompile(`(?i) line (\d+).*script (?:in )?(\S+)(?:: in function (\S+))?$`)
	// Opera 11+ stacktrace property format (with column)
	opera11StacktraceWithColumnRE = regexp.MustCompile(`(?i) line (\d+), column (\d+)\s*(?:in (?:<anonymous function: ([^>]+)>|([^\)]+))\((.*)\))? in (.*):\s*$`)

	// Opera 9 and earlier (linked script format)
	lineRE1 = regexp.MustCompile(`(?i)^\s*Line (\d+) of linked script ((?:file|https?|blob)\S+)(?:: in function (\S+))?\s*$`)
	// Opera 9 and earlier (inline script format)
	lineRE2 = regexp.MustCompile(`(?i)^\s*Line (\d+) of inline#(\d+) script in ((?:file|https?|blob)\S+)(?:: in function (\S+))?\s*$`)
	// Opera 9 and earlier (function script format)
	lineRE3 = regexp.MustCompile(`(?i)^\s*Line (\d+) of function script\s*$`)
)

// parseMode indicates which parsing strategy successfully extracted the stack trace.
type parseMode string

const (
	// parseModeStack indicates the stack was parsed using the standard format
	// from modern browsers (Chrome V8, Firefox Gecko, Safari WebKit, Edge).
	parseModeStack parseMode = "stack"

	// parseModeStacktrace indicates the stack was parsed using the Opera 10-12 format
	// which has a distinct structure with "line N, column M" patterns.
	parseModeStacktrace parseMode = "stacktrace"

	// parseModeMultiline indicates the stack was parsed using Opera 9's multiline format
	// where frames are embedded within the error message text.
	parseModeMultiline parseMode = "multiline"
)

// stackFrame represents a single frame in a stack trace.
type stackFrame struct {
	url      string
	funcName string
	line     *int
	column   *int
}

// stackTrace represents a complete JavaScript stack trace.
type stackTrace struct {
	name        string
	message     string
	mode        parseMode
	stackFrames []stackFrame
}

// computeStackTraceFromStackProp parses stack traces from the stack property (Chrome/Gecko).
func computeStackTraceFromStackProp(name, message, stack string) *stackTrace {
	if stack == "" {
		return nil
	}

	lines := strings.Split(stack, "\n")
	stackFrames := []stackFrame{}

	for _, line := range lines {
		var element *stackFrame

		// Try React Native format first (more specific pattern)
		if matches := reactNativeRE.FindStringSubmatch(line); matches != nil {
			element = &stackFrame{
				url:      matches[2],
				funcName: matches[1],
			}

			if lineInt, err := strconv.Atoi(matches[3]); err == nil {
				element.line = &lineInt
			}
			if colInt, err := strconv.Atoi(matches[4]); err == nil {
				element.column = &colInt
			}

			if element.funcName == "" {
				element.funcName = unknownFunction
			}
		} else if matches := chromeRE.FindStringSubmatch(line); matches != nil {
			// Try Chrome format
			isNative := strings.HasPrefix(matches[2], "native")
			isEval := strings.HasPrefix(matches[2], "eval")

			url := matches[2]
			lineNo := matches[3]
			col := matches[4]

			if isEval {
				if evalMatches := chromeEvalRE.FindStringSubmatch(matches[2]); evalMatches != nil {
					url = evalMatches[1]
					lineNo = evalMatches[2]
					col = evalMatches[3]
				}
			}

			if isNative {
				url = "(native)"
			}

			element = &stackFrame{
				url:      url,
				funcName: matches[1],
			}

			if lineInt, err := strconv.Atoi(lineNo); err == nil {
				element.line = &lineInt
			}
			if colInt, err := strconv.Atoi(col); err == nil {
				element.column = &colInt
			}

			if element.funcName == "" {
				element.funcName = unknownFunction
			}
		} else if matches := winJSRE.FindStringSubmatch(line); matches != nil {
			// Try WinJS format
			element = &stackFrame{
				url:      matches[2],
				funcName: matches[1],
			}
			if lineInt, err := strconv.Atoi(matches[3]); err == nil {
				element.line = &lineInt
			}
			if colInt, err := strconv.Atoi(matches[4]); err == nil {
				element.column = &colInt
			}
			if element.funcName == "" {
				element.funcName = unknownFunction
			}
		} else if matches := geckoRE.FindStringSubmatch(line); matches != nil {
			// Try Gecko format
			isEval := strings.Contains(matches[3], " > eval")
			lineNo := matches[4]
			col := matches[5]

			if isEval {
				if evalMatches := geckoEvalRE.FindStringSubmatch(matches[3]); evalMatches != nil {
					matches[3] = evalMatches[1]
					lineNo = evalMatches[2]
					col = ""
				}
			}

			element = &stackFrame{
				url:      matches[3],
				funcName: matches[1],
			}

			if lineInt, err := strconv.Atoi(lineNo); err == nil {
				element.line = &lineInt
			}
			if colInt, err := strconv.Atoi(col); err == nil {
				element.column = &colInt
			}

			if element.funcName == "" {
				element.funcName = unknownFunction
			}
		} else {
			continue
		}

		stackFrames = append(stackFrames, *element)
	}

	if len(stackFrames) == 0 {
		return nil
	}

	return &stackTrace{
		name:        name,
		message:     message,
		mode:        parseModeStack,
		stackFrames: stackFrames,
	}
}

// computeStackTraceFromOpera11Stacktrace parses Opera 11+ stacktrace property.
func computeStackTraceFromOpera11Stacktrace(name, message, stacktrace string) *stackTrace {
	if stacktrace == "" {
		return nil
	}

	lines := strings.Split(stacktrace, "\n")
	stackFrames := []stackFrame{}

	for i := 0; i < len(lines); i += 2 {
		var element *stackFrame

		if matches := opera11StacktraceRE.FindStringSubmatch(lines[i]); matches != nil {
			funcName := matches[3]
			if funcName == "" {
				funcName = unknownFunction
			}
			element = &stackFrame{
				url:      matches[2],
				funcName: funcName,
			}
			if lineInt, err := strconv.Atoi(matches[1]); err == nil {
				element.line = &lineInt
			}
		} else if matches := opera11StacktraceWithColumnRE.FindStringSubmatch(lines[i]); matches != nil {
			func1, func2 := matches[3], matches[4]
			func_ := func1
			if func_ == "" {
				func_ = func2
			}
			if func_ == "" {
				func_ = unknownFunction
			}

			element = &stackFrame{
				url:      matches[6],
				funcName: func_,
			}
			if lineInt, err := strconv.Atoi(matches[1]); err == nil {
				element.line = &lineInt
			}
			if colInt, err := strconv.Atoi(matches[2]); err == nil {
				element.column = &colInt
			}
		}

		if element != nil {
			stackFrames = append(stackFrames, *element)
		}
	}

	if len(stackFrames) == 0 {
		return nil
	}

	return &stackTrace{
		name:        name,
		message:     message,
		mode:        parseModeStacktrace,
		stackFrames: stackFrames,
	}
}

// computeStackTraceFromOpera9Message parses Opera 9 message property.
func computeStackTraceFromOpera9Message(name, message string) *stackTrace {
	lines := strings.Split(message, "\n")
	if len(lines) < 4 {
		return nil
	}

	stackFrames := []stackFrame{}

	for line := 2; line < len(lines); line += 2 {
		var item *stackFrame

		if matches := lineRE1.FindStringSubmatch(lines[line]); matches != nil {
			funcName := matches[3]
			if funcName == "" {
				funcName = unknownFunction
			}
			item = &stackFrame{
				url:      matches[2],
				funcName: funcName,
			}
			if lineInt, err := strconv.Atoi(matches[1]); err == nil {
				item.line = &lineInt
			}
		} else if matches := lineRE2.FindStringSubmatch(lines[line]); matches != nil {
			funcName := matches[4]
			if funcName == "" {
				funcName = unknownFunction
			}
			item = &stackFrame{
				url:      matches[3],
				funcName: funcName,
			}
			if lineInt, err := strconv.Atoi(matches[1]); err == nil {
				item.line = &lineInt
			}
		} else if matches := lineRE3.FindStringSubmatch(lines[line]); matches != nil {
			item = &stackFrame{
				url:      "",
				funcName: unknownFunction,
				line:     nil,
				column:   nil,
			}
		}

		if item != nil {
			stackFrames = append(stackFrames, *item)
		}
	}

	if len(stackFrames) == 0 {
		return nil
	}

	return &stackTrace{
		name:        name,
		message:     lines[0],
		mode:        parseModeMultiline,
		stackFrames: stackFrames,
	}
}

// computeStackTraceFromOpera10Stacktrace parses Opera 10 stacktrace property (uses Opera 9 format).
func computeStackTraceFromOpera10Stacktrace(name, message, stacktrace string) *stackTrace {
	lines := strings.Split(stacktrace, "\n")
	if len(lines) < 2 {
		return nil
	}

	stackFrames := []stackFrame{}

	for line := 0; line < len(lines); line += 2 {
		var item *stackFrame

		if matches := lineRE1.FindStringSubmatch(lines[line]); matches != nil {
			funcName := matches[3]
			if funcName == "" {
				funcName = unknownFunction
			}
			item = &stackFrame{
				url:      matches[2],
				funcName: funcName,
			}
			if lineInt, err := strconv.Atoi(matches[1]); err == nil {
				item.line = &lineInt
			}
		} else if matches := lineRE2.FindStringSubmatch(lines[line]); matches != nil {
			funcName := matches[4]
			if funcName == "" {
				funcName = unknownFunction
			}
			item = &stackFrame{
				url:      matches[3],
				funcName: funcName,
			}
			if lineInt, err := strconv.Atoi(matches[1]); err == nil {
				item.line = &lineInt
			}
		} else if matches := lineRE3.FindStringSubmatch(lines[line]); matches != nil {
			item = &stackFrame{
				url:      "",
				funcName: unknownFunction,
				line:     nil,
				column:   nil,
			}
		}

		if item != nil {
			stackFrames = append(stackFrames, *item)
		}
	}

	if len(stackFrames) == 0 {
		return nil
	}

	return &stackTrace{
		name:        name,
		message:     message,
		mode:        parseModeStacktrace,
		stackFrames: stackFrames,
	}
}

// computeStackTrace parses a JavaScript error stack trace.
// It tries multiple parsing strategies based on the stack trace format.
// Returns an error if all parsing strategies fail.
func computeStackTrace(name, message, stack string) (*stackTrace, error) {
	var result *stackTrace

	if stack != "" {
		// Try Opera 11+ stacktrace property
		result = computeStackTraceFromOpera11Stacktrace(name, message, stack)
		if result != nil {
			return result, nil
		}

		// Try stack property (Chrome/Gecko)
		result = computeStackTraceFromStackProp(name, message, stack)
		if result != nil {
			return result, nil
		}

		// Try Opera 10 stacktrace property (uses Opera 9 format)
		result = computeStackTraceFromOpera10Stacktrace(name, message, stack)
		if result != nil {
			return result, nil
		}
	}

	// Try Opera 9 message property
	result = computeStackTraceFromOpera9Message(name, message)
	if result != nil {
		return result, nil
	}

	// Fallback if parsing failed
	return nil, fmt.Errorf("failed to parse stack trace")
}
