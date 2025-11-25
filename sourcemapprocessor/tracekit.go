package sourcemapprocessor

import (
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

const (
	UnknownFunction = "?"
)

// StackFrame represents a single frame in a stack trace.
type StackFrame struct {
	URL     string
	Func    string
	Args    []string
	Line    *int
	Column  *int
	Context []string
}

// StackTrace represents a complete JavaScript stack trace.
type StackTrace struct {
	Name       string
	Message    string
	Mode       string // 'stack', 'stacktrace', 'multiline', 'callers', 'onerror', or 'failed'
	Stack      []StackFrame
	Incomplete bool
	Partial    bool
}

// TraceKit provides methods for parsing JavaScript stack traces.
type TraceKit struct {
	remoteFetching bool
	linesOfContext int
	sourceCache    map[string][]string
	httpClient     *http.Client
}

// NewTraceKit creates a new TraceKit instance.
func NewTraceKit() *TraceKit {
	return &TraceKit{
		remoteFetching: true,
		linesOfContext: 11,
		sourceCache:    make(map[string][]string),
		httpClient:     &http.Client{},
	}
}

// LoadSource attempts to fetch source code from a URL via HTTP.
func (tk *TraceKit) LoadSource(url string) string {
	if !tk.remoteFetching {
		return ""
	}

	resp, err := tk.httpClient.Get(url)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	return string(body)
}

// GetSource retrieves source code from cache or fetches it.
func (tk *TraceKit) GetSource(url string) []string {
	if url == "" {
		return []string{}
	}

	if cached, ok := tk.sourceCache[url]; ok {
		return cached
	}

	source := tk.LoadSource(url)
	lines := []string{}
	if source != "" {
		lines = strings.Split(source, "\n")
	}

	tk.sourceCache[url] = lines
	return lines
}

// GuessFunctionName attempts to determine the function name from source code.
func (tk *TraceKit) GuessFunctionName(url string, lineNo int) string {
	reFunctionArgNames := regexp.MustCompile(`function ([^(]*)\(([^)]*)\)`)
	reGuessFunction := regexp.MustCompile(`['"]?([0-9A-Za-z$_]+)['"]?\s*[:=]\s*(function|eval|new Function)`)

	source := tk.GetSource(url)
	if len(source) == 0 {
		return UnknownFunction
	}

	maxLines := 10
	line := ""

	// Walk backwards from the line in the function until we find the function definition
	for i := 0; i < maxLines && lineNo-i > 0; i++ {
		if lineNo-i-1 < len(source) {
			line = source[lineNo-i-1] + line
		}

		if matches := reGuessFunction.FindStringSubmatch(line); matches != nil {
			return matches[1]
		}

		if matches := reFunctionArgNames.FindStringSubmatch(line); matches != nil {
			return matches[1]
		}
	}

	return UnknownFunction
}

// GatherContext retrieves surrounding lines of source code for context.
func (tk *TraceKit) GatherContext(url string, line int) []string {
	source := tk.GetSource(url)
	if len(source) == 0 {
		return nil
	}

	linesBefore := tk.linesOfContext / 2
	linesAfter := linesBefore + (tk.linesOfContext % 2)

	start := line - linesBefore - 1
	if start < 0 {
		start = 0
	}

	end := line + linesAfter - 1
	if end > len(source) {
		end = len(source)
	}

	context := []string{}
	for i := start; i < end; i++ {
		if i >= 0 && i < len(source) {
			context = append(context, source[i])
		}
	}

	if len(context) > 0 {
		return context
	}
	return nil
}

// EscapeRegExp escapes special regex characters in a string.
func (tk *TraceKit) EscapeRegExp(text string) string {
	special := []string{`\`, `[`, `]`, `{`, `}`, `(`, `)`, `*`, `+`, `?`, `.`, `,`, `^`, `$`, `|`, `#`, `-`}
	result := text
	for _, char := range special {
		result = strings.ReplaceAll(result, char, `\`+char)
	}
	return result
}

// FindSourceInLine finds the column position of a code fragment in a source line.
func (tk *TraceKit) FindSourceInLine(fragment, url string, line int) *int {
	source := tk.GetSource(url)
	if len(source) < line || line < 1 {
		return nil
	}

	re, err := regexp.Compile(`\b` + tk.EscapeRegExp(fragment) + `\b`)
	if err != nil {
		return nil
	}

	if matches := re.FindStringIndex(source[line-1]); matches != nil {
		col := matches[0]
		return &col
	}

	return nil
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
	referenceRE := regexp.MustCompile(`^(.*) is undefined$`)

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
				Args: []string{},
			}

			if isNative && matches[2] != "" {
				element.Args = []string{matches[2]}
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
				URL:    matches[2],
				Func:   matches[1],
				Args:   []string{},
				Line:   &lineInt,
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
				Args: []string{},
			}

			if matches[2] != "" {
				element.Args = strings.Split(matches[2], ",")
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
			// Try to guess function name if not found
			if element.Func == UnknownFunction && element.Line != nil {
				element.Func = tk.GuessFunctionName(element.URL, *element.Line)
			}

			// Gather context
			if element.Line != nil {
				element.Context = tk.GatherContext(element.URL, *element.Line)
			}

			stackFrames = append(stackFrames, *element)
		}
	}

	if len(stackFrames) == 0 {
		return nil
	}

	// Try to find column in reference if we have one
	if len(stackFrames) > 0 && stackFrames[0].Line != nil && stackFrames[0].Column == nil {
		if matches := referenceRE.FindStringSubmatch(message); matches != nil {
			col := tk.FindSourceInLine(matches[1], stackFrames[0].URL, *stackFrames[0].Line)
			if col != nil {
				stackFrames[0].Column = col
			}
		}
	}

	return &StackTrace{
		Name:    name,
		Message: message,
		Mode:    "stack",
		Stack:   stackFrames,
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
				URL:    matches[2],
				Line:   &lineInt,
				Column: nil,
				Func:   matches[3],
				Args:   []string{},
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
				Line:   &lineInt,
				Column: &colInt,
				Func:   func_,
				Args:   []string{},
			}
			if matches[5] != "" {
				element.Args = strings.Split(matches[5], ",")
			}
		}

		if element != nil {
			if element.Func == "" && element.Line != nil {
				element.Func = tk.GuessFunctionName(element.URL, *element.Line)
			}

			if element.Line != nil {
				element.Context = tk.GatherContext(element.URL, *element.Line)
			}

			if element.Context == nil && i+1 < len(lines) {
				element.Context = []string{lines[i+1]}
			}

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
		Stack:   stackFrames,
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
				URL:    matches[2],
				Func:   matches[3],
				Args:   []string{},
				Line:   &lineInt,
				Column: nil,
			}
		} else if matches := lineRE2.FindStringSubmatch(lines[line]); matches != nil {
			lineInt := parseInt(matches[1])
			item = &StackFrame{
				URL:    matches[3],
				Func:   matches[4],
				Args:   []string{},
				Line:   &lineInt,
				Column: nil,
			}
		} else if matches := lineRE3.FindStringSubmatch(lines[line]); matches != nil {
			item = &StackFrame{
				URL:    "",
				Func:   "",
				Args:   []string{},
				Line:   nil,
				Column: nil,
			}
		}

		if item != nil {
			if item.Func == "" && item.Line != nil {
				item.Func = tk.GuessFunctionName(item.URL, *item.Line)
			}
			if item.Line != nil {
				item.Context = tk.GatherContext(item.URL, *item.Line)
			}
			if item.Context == nil && line+1 < len(lines) {
				item.Context = []string{lines[line+1]}
			}
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
		Stack:   stackFrames,
	}
}

// ComputeStackTrace parses a JavaScript error stack trace.
// It tries multiple parsing strategies based on the stack trace format.
func (tk *TraceKit) ComputeStackTrace(name, message, stack string, depth int) *StackTrace {
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
		Stack:   []StackFrame{},
	}
}

// Helper function to parse integer strings
func parseInt(s string) int {
	val, _ := strconv.Atoi(s)
	return val
}