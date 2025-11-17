package proguardprocessor

import (
	"errors"
	"regexp"
	"strconv"
	"strings"
)

var (
	errEmptyStackTrace = errors.New("stack trace is empty")
	errInvalidStackTrace = errors.New("invalid stack trace format")
	errNoFramesParsed  = errors.New("no valid stack frames found in stack trace")
)

// stackFrame represents a single frame in a stack trace.
type stackFrame struct {
	class      string
	method     string
	line       int
	sourceFile string
}

// element represents a single element in a stack trace.
// Either a frame or a raw line can be stored. Not both at the same time.
type element struct {
	frame stackFrame
	line  string
}

// stackTrace represents the parsed stack trace.
type stackTrace struct {
	exceptionType    string
	exceptionMessage string
	elements         []element
}

// Regex patterns for parsing stack traces.
var (
	// exceptionHeaderRegex matches the first line of a stack trace to extract
	// the exception type and message.
	// Capture Groups:
	// 		1: Exception type
	// 		2: Exception message
	//
	// Examples that match:
	// 		foo.bar.baz:literally anything after colon
	// 		foo.bar.baz		:	  literally anything after colon
	// 		foo  :  literally anything after colon
	//
	exceptionHeaderRegex = regexp.MustCompile(`^([^\s:]+)\s*:\s*(.*)$`)
	// stackFrameRegex matches exception stack frames.
	// Capture Groups:
	// 		1: Full class name
	// 		2: Method name
	// 		3: Source name
	// 		4: Line number (optional)
	//
	// Examples that match:
	// 		at com.example.Class.method(File.java:123)
	// 		at com.example.Class.method(File.java:-2)
	// 		at com.example.Class.method(Native Method)
	// 		at com.example.Class.method(Unknown Source)
	// 		at com.example.Class.method(File.java)
	//
	stackFrameRegex = regexp.MustCompile(`^\s*at\s+([^\s(]+)\.([^\s.(]+)\(([^:)]+)(?::(-?\d+))?\)\s*$`)
)

// parseStackTrace parses a raw stack trace string into structured components.
// Stack trace string is parsed into exception type, message, and stack frames.
// Returns an error if the stack trace is empty or contains no valid frames.
func parseStackTrace(stackTraceStr string) (*stackTrace, error) {
	if stackTraceStr == "" {
		return nil, errEmptyStackTrace
	}

	lines := strings.Split(stackTraceStr, "\n")
	if len(lines) == 0 {
		return nil, errEmptyStackTrace
	}

	result := &stackTrace{
		frames: make([]stackFrame, 0),
	}

	// Parse the first line to extract exception type and message
	firstLine := strings.TrimSpace(lines[0])
	if firstLine != "" && exceptionHeaderRegex.MatchString(firstLine) {
		matches := exceptionHeaderRegex.FindStringSubmatch(firstLine)
		result.exceptionType = matches[1]
		result.exceptionMessage = matches[2]
	} else {
		return nil, errInvalidStackTrace
	}

	// Parse each subsequent line as a stack frame
	for i := 1; i < len(lines); i++ {
		line := lines[i]

		// Skip empty lines
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Skip "Caused by:" lines and similar
		if strings.Contains(line, "Caused by:") || strings.Contains(line, "Suppressed:") {
			// We could extend this to handle caused-by chains in the future
			break
		}

		frame := parseStackFrame(line)
		if frame != nil {
			result.frames = append(result.frames, *frame)
		}
	}

	// If we didn't parse any frames, return an error
	if len(result.frames) == 0 {
		return nil, errNoFramesParsed
	}

	return result, nil
}

// parseStackFrame parses a single stack frame string into a stackFrame struct.
// Returns nil if the line cannot be parsed as a valid stack frame.
func parseStackFrame(line string) *stackFrame {
	matches := stackFrameRegex.FindStringSubmatch(line)
	if matches == nil {
		return nil
	}

	// Extract the class and method names
	className := matches[1]
	methodName := matches[2]
	sourceInfo := matches[3]
	lineNumStr := matches[4]

	frame := &stackFrame{
		class:      className,
		method:     methodName,
		sourceFile: sourceInfo,
		line:       -1, // Default to -1
	}

	// Handle special source info values
	// Based on https://developer.android.com/reference/java/lang/StackTraceElement#StackTraceElement(java.lang.String,%20java.lang.String,%20java.lang.String,%20int)
	if sourceInfo == "Native Method" {
		frame.line = -2 // Android convention for native methods
	} else if lineNumStr != "" { // Parse line number if present
		if lineNum, err := strconv.Atoi(lineNumStr); err == nil {
			frame.line = lineNum
		}
	}

	return frame
}