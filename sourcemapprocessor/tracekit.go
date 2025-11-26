package sourcemapprocessor

import (
	"regexp"
	"strconv"
	"strings"
)

const (
	UnknownFunction = "?"
)

// StackFrame represents a single frame in a stack trace.
type StackFrame struct {
	URL    string
	Func   string
	Line   *int
	Column *int
}

// StackTrace represents a complete JavaScript stack trace.
type StackTrace struct {
	Name       string
	Message    string
	Mode       string // 'stack', 'stacktrace', 'multiline', or 'failed'
	StackFrames      []StackFrame
}

// TraceKit provides methods for parsing JavaScript stack traces.
type TraceKit struct{}

// NewTraceKit creates a new TraceKit instance.
func NewTraceKit() *TraceKit {
	return &TraceKit{}
}

// ComputeStackTraceFromStackProp parses stack traces from the stack property (Chrome/Gecko).
func (tk *TraceKit) ComputeStackTraceFromStackProp(name, message, stack string) *StackTrace {
	if stack == "" {
		return nil
	}

	chromeRE := regexp.MustCompile(`^\s*at (.*?) ?\(((?:file|https?|blob|chrome-extension|native|eval|webpack|<anonymous>|\/).*?)(?::(\d+))?(?::(\d+))?\)?\s*$`)
	geckoRE := regexp.MustCompile(`^\s*(.*?)(?:\((.*?)\))?(?:^|@)((?:file|https?|blob|chrome|webpack|resource|\[native).*?|[^@]*bundle)(?::(\d+))?(?::(\d+))?\s*$`)
	winJSRE := regexp.MustCompile(`^\s*at (?:((?:\[object object\])?.+) )?\(?((?:file|ms-appx|https?|webpack|blob):.*?):(\d+)(?::(\d+))?\)?\s*$`)
	geckoEvalRE := regexp.MustCompile(`(\S+) line (\d+)(?: > eval line \d+)* > eval`)
	chromeEvalRE := regexp.MustCompile(`\((\S*)(?::(\d+))(?::(\d+))\)`)

	lines := strings.Split(stack, "\n")
	stackFrames := []StackFrame{}

	for _, line := range lines {
		var element *StackFrame

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

			element = &StackFrame{
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
				element.Func = UnknownFunction
			}
		} else if matches := winJSRE.FindStringSubmatch(line); matches != nil {
			// Try WinJS format
			lineInt := parseInt(matches[3])
			element = &StackFrame{
				URL:  matches[2],
				Func: matches[1],
				Line: &lineInt,
				Column: nil,
			}
			if matches[4] != "" {
				colInt := parseInt(matches[4])
				element.Column = &colInt
			}
			if element.Func == "" {
				element.Func = UnknownFunction
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

			element = &StackFrame{
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
				element.Func = UnknownFunction
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

	return &StackTrace{
		Name:    name,
		Message: message,
		Mode:    "stack",
		StackFrames:   stackFrames,
	}
}

// ComputeStackTraceFromStacktraceProp parses stack traces from stacktrace property (Opera 10+).
func (tk *TraceKit) ComputeStackTraceFromStacktraceProp(name, message, stacktrace string) *StackTrace {
	if stacktrace == "" {
		return nil
	}

	opera10RE := regexp.MustCompile(`line (\d+).*script (?:in )?(\S+)(?:: in function (\S+))?$`)
	opera11RE := regexp.MustCompile(`line (\d+), column (\d+)\s*(?:in (?:<anonymous function: ([^>]+)>|([^\)]+))\((.*)\))? in (.*):\s*$`)

	lines := strings.Split(stacktrace, "\n")
	stackFrames := []StackFrame{}

	for i := 0; i < len(lines); i += 2 {
		var element *StackFrame

		if matches := opera10RE.FindStringSubmatch(lines[i]); matches != nil {
			lineInt := parseInt(matches[1])
			element = &StackFrame{
				URL:  matches[2],
				Func: matches[3],
				Line: &lineInt,
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

			element = &StackFrame{
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

	return &StackTrace{
		Name:    name,
		Message: message,
		Mode:    "stacktrace",
		StackFrames:   stackFrames,
	}
}

// ComputeStackTraceFromOperaMultiLineMessage parses Opera 9 and earlier stack traces.
func (tk *TraceKit) ComputeStackTraceFromOperaMultiLineMessage(name, message string) *StackTrace {
	lines := strings.Split(message, "\n")
	if len(lines) < 4 {
		return nil
	}

	lineRE1 := regexp.MustCompile(`^\s*Line (\d+) of linked script ((?:file|https?|blob)\S+)(?:: in function (\S+))?\s*$`)
	lineRE2 := regexp.MustCompile(`^\s*Line (\d+) of inline#(\d+) script in ((?:file|https?|blob)\S+)(?:: in function (\S+))?\s*$`)
	lineRE3 := regexp.MustCompile(`^\s*Line (\d+) of function script\s*$`)

	stackFrames := []StackFrame{}

	for line := 2; line < len(lines); line += 2 {
		var item *StackFrame

		if matches := lineRE1.FindStringSubmatch(lines[line]); matches != nil {
			lineInt := parseInt(matches[1])
			item = &StackFrame{
				URL:  matches[2],
				Func: matches[3],
				Line: &lineInt,
				Column: nil,
			}
		} else if matches := lineRE2.FindStringSubmatch(lines[line]); matches != nil {
			lineInt := parseInt(matches[1])
			item = &StackFrame{
				URL:  matches[3],
				Func: matches[4],
				Line: &lineInt,
				Column: nil,
			}
		} else if matches := lineRE3.FindStringSubmatch(lines[line]); matches != nil {
			item = &StackFrame{
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

	return &StackTrace{
		Name:    name,
		Message: lines[0],
		Mode:    "multiline",
		StackFrames:   stackFrames,
	}
}

// ComputeStackTrace parses a JavaScript error stack trace.
// It tries multiple parsing strategies based on the stack trace format.
func (tk *TraceKit) ComputeStackTrace(name, message, stack string) *StackTrace {
	var result *StackTrace

	// Try stacktrace property first (Opera 10+)
	if stack != "" {
		result = tk.ComputeStackTraceFromStacktraceProp(name, message, stack)
		if result != nil {
			return result
		}

		// Try stack property (Chrome/Gecko)
		result = tk.ComputeStackTraceFromStackProp(name, message, stack)
		if result != nil {
			return result
		}

		// Try Opera multiline message format
		result = tk.ComputeStackTraceFromOperaMultiLineMessage(name, message)
		if result != nil {
			return result
		}
	}

	// Fallback if parsing failed
	return &StackTrace{
		Name:    name,
		Message: message,
		Mode:    "failed",
		StackFrames:   []StackFrame{},
	}
}

// Helper function to parse integer strings
func parseInt(s string) int {
	val, _ := strconv.Atoi(s)
	return val
}