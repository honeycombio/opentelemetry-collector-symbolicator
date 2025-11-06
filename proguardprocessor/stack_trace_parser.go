package proguardprocessor

import (
	"regexp"
)

// stackFrame represents a single frame in a stack trace.
type stackFrame struct {
	class string
	method string
	line   int
	sourceFile string
}

// stackTrace represents the parsed stack trace.
type stackTrace struct {
	exceptionType string
	exceptionMessage string
	frames []stackFrame
}

// Regex patterns for parsing stack traces.
var (
	exceptionHeaderRegex = regexp.MustCompile(`^([^\s:]+(?:\.[^\s:]+)*)\s*:\s*(.*)$`)
	// According to Claude, this should match the following formats:
	// Matches lines like: at com.example.Class.method(File.java:123)
	// Also matches: at com.example.Class.method(Native Method)
	// Also matches: at com.example.Class.method(Unknown Source)
	// Also matches: at com.example.Class.method(File.java)
	stackFrameRegex      = regexp.MustCompile(`^\s*at\s+([^\s(]+)\.([^\s.(]+)\(([^:)]+)(?::(\d+))?\)\s*$`)
)

