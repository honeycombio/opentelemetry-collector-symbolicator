// This code was originally adapted from TraceKit, a JavaScript error tracing and
// stack trace parsing library.
// TraceKit is MIT-licensed and available at: https://github.com/csnover/TraceKit

package sourcemapprocessor

import (
	"regexp"
	"strconv"
	"strings"
)

const (
	unknownFunction = "?"
)

// Compiled regular expressions for parsing stack traces from various browsers.
var (
	// Chrome/V8 stack trace format
	chromeRE = regexp.MustCompile(`^\s*at (.*?) ?\(((?:file|https?|blob|chrome-extension|native|eval|webpack|<anonymous>|\/).*?)(?::(\d+))?(?::(\d+))?\)?\s*$`)
	// Gecko/Firefox stack trace format
	geckoRE = regexp.MustCompile(`^\s*(.*?)(?:\((.*?)\))?(?:^|@)((?:file|https?|blob|chrome|webpack|resource|\[native).*?|[^@]*bundle)(?::(\d+))?(?::(\d+))?\s*$`)
	// Windows JavaScript (WinJS) stack trace format
	winJSRE = regexp.MustCompile(`^\s*at (?:((?:\[object object\])?.+) )?\(?((?:file|ms-appx|https?|webpack|blob):.*?):(\d+)(?::(\d+))?\)?\s*$`)
	// Gecko eval format
	geckoEvalRE = regexp.MustCompile(`(\S+) line (\d+)(?: > eval line \d+)* > eval`)
	// Chrome eval format
	chromeEvalRE = regexp.MustCompile(`\((\S*)(?::(\d+))(?::(\d+))\)`)

	// Opera 10 stack trace format
	opera10RE = regexp.MustCompile(`line (\d+).*script (?:in )?(\S+)(?:: in function (\S+))?$`)
	// Opera 11+ stack trace format
	opera11RE = regexp.MustCompile(`line (\d+), column (\d+)\s*(?:in (?:<anonymous function: ([^>]+)>|([^\)]+))\((.*)\))? in (.*):\s*$`)

	// Opera 9 and earlier (linked script format)
	lineRE1 = regexp.MustCompile(`^\s*Line (\d+) of linked script ((?:file|https?|blob)\S+)(?:: in function (\S+))?\s*$`)
	// Opera 9 and earlier (inline script format)
	lineRE2 = regexp.MustCompile(`^\s*Line (\d+) of inline#(\d+) script in ((?:file|https?|blob)\S+)(?:: in function (\S+))?\s*$`)
	// Opera 9 and earlier (function script format)
	lineRE3 = regexp.MustCompile(`^\s*Line (\d+) of function script\s*$`)
)

// stackFrame represents a single frame in a stack trace.
type stackFrame struct {
	URL    string
	Func   string
	Line   *int
	Column *int
}

// stackTrace represents a complete JavaScript stack trace.
type stackTrace struct {
	Name        string
	Message     string
	Mode        string // 'stack', 'stacktrace', 'multiline', or 'failed'
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

		// Try Chrome format
		if matches := chromeRE.FindStringSubmatch(line); matches != nil {
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
				url = ""
			}

			element = &stackFrame{
				URL:  url,
				Func: matches[1],
			}

			if lineNo != "" {
				lineInt := parseInt(lineNo)
				element.Line = &lineInt
			}
			if col != "" {
				colInt := parseInt(col)
				element.Column = &colInt
			}

			if element.Func == "" {
				element.Func = unknownFunction
			}
		} else if matches := winJSRE.FindStringSubmatch(line); matches != nil {
			// Try WinJS format
			lineInt := parseInt(matches[3])
			element = &stackFrame{
				URL:    matches[2],
				Func:   matches[1],
				Line:   &lineInt,
				Column: nil,
			}
			if matches[4] != "" {
				colInt := parseInt(matches[4])
				element.Column = &colInt
			}
			if element.Func == "" {
				element.Func = unknownFunction
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
				URL:  matches[3],
				Func: matches[1],
			}

			if lineNo != "" {
				lineInt := parseInt(lineNo)
				element.Line = &lineInt
			}
			if col != "" {
				colInt := parseInt(col)
				element.Column = &colInt
			}

			if element.Func == "" {
				element.Func = unknownFunction
			}
		} else {
			continue
		}

		if element != nil {
			stackFrames = append(stackFrames, *element)
		}
	}

	if len(stackFrames) == 0 {
		return nil
	}

	return &stackTrace{
		Name:        name,
		Message:     message,
		Mode:        "stack",
		stackFrames: stackFrames,
	}
}

// computeStackTraceFromStacktraceProp parses stack traces from stacktrace property (Opera 10+).
func computeStackTraceFromStacktraceProp(name, message, stacktrace string) *stackTrace {
	if stacktrace == "" {
		return nil
	}

	lines := strings.Split(stacktrace, "\n")
	stackFrames := []stackFrame{}

	for i := 0; i < len(lines); i += 2 {
		var element *stackFrame

		if matches := opera10RE.FindStringSubmatch(lines[i]); matches != nil {
			lineInt := parseInt(matches[1])
			element = &stackFrame{
				URL:    matches[2],
				Func:   matches[3],
				Line:   &lineInt,
				Column: nil,
			}
		} else if matches := opera11RE.FindStringSubmatch(lines[i]); matches != nil {
			lineInt := parseInt(matches[1])
			colInt := parseInt(matches[2])
			func1, func2 := matches[3], matches[4]
			func_ := func1
			if func_ == "" {
				func_ = func2
			}

			element = &stackFrame{
				URL:    matches[6],
				Func:   func_,
				Line:   &lineInt,
				Column: &colInt,
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
		Name:        name,
		Message:     message,
		Mode:        "stacktrace",
		stackFrames: stackFrames,
	}
}

// computeStackTraceFromOperaMultiLineMessage parses Opera 9 and earlier stack traces.
func computeStackTraceFromOperaMultiLineMessage(name, message string) *stackTrace {
	lines := strings.Split(message, "\n")
	if len(lines) < 4 {
		return nil
	}

	stackFrames := []stackFrame{}

	for line := 2; line < len(lines); line += 2 {
		var item *stackFrame

		if matches := lineRE1.FindStringSubmatch(lines[line]); matches != nil {
			lineInt := parseInt(matches[1])
			item = &stackFrame{
				URL:    matches[2],
				Func:   matches[3],
				Line:   &lineInt,
				Column: nil,
			}
		} else if matches := lineRE2.FindStringSubmatch(lines[line]); matches != nil {
			lineInt := parseInt(matches[1])
			item = &stackFrame{
				URL:    matches[3],
				Func:   matches[4],
				Line:   &lineInt,
				Column: nil,
			}
		} else if matches := lineRE3.FindStringSubmatch(lines[line]); matches != nil {
			item = &stackFrame{
				URL:    "",
				Func:   "",
				Line:   nil,
				Column: nil,
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
		Name:        name,
		Message:     lines[0],
		Mode:        "multiline",
		stackFrames: stackFrames,
	}
}

// computeStackTrace parses a JavaScript error stack trace.
// It tries multiple parsing strategies based on the stack trace format.
func computeStackTrace(name, message, stack string) *stackTrace {
	var result *stackTrace

	// Try stacktrace property first (Opera 10+)
	if stack != "" {
		result = computeStackTraceFromStacktraceProp(name, message, stack)
		if result != nil {
			return result
		}

		// Try stack property (Chrome/Gecko)
		result = computeStackTraceFromStackProp(name, message, stack)
		if result != nil {
			return result
		}

		// Try Opera multiline message format
		result = computeStackTraceFromOperaMultiLineMessage(name, message)
		if result != nil {
			return result
		}
	}

	// Fallback if parsing failed
	return &stackTrace{
		Name:        name,
		Message:     message,
		Mode:        "failed",
		stackFrames: []stackFrame{},
	}
}

// Helper function to parse integer strings
func parseInt(s string) int {
	val, _ := strconv.Atoi(s)
	return val
}
