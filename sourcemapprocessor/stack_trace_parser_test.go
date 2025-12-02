package sourcemapprocessor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intPtr(i int) *int {
	return &i
}

func TestTraceKit(t *testing.T) {
	line10 := 10
	col15 := 15
	line20 := 20
	col25 := 25

	tests := []struct {
		name            string
		exceptionName   string
		exceptionMsg    string
		stack           string
		expectedName    string
		expectedMessage string
		expectedFrames  []stackFrame
		expectedMode    string
	}{
		{
			name:          "Valid TraceKit stack trace with provided exception info",
			exceptionName: "Error",
			exceptionMsg:  "Something went wrong",
			stack: "" +
				"Error: Something went wrong\n" +
				"    at funcA (fileA.js:10:15)\n" + // stack[0]
				"    at funcB (fileB.js:20:25)", // stack[1]
			expectedName:    "Error",
			expectedMessage: "Something went wrong",
			expectedFrames: []stackFrame{
				{url: "fileA.js", funcName: "funcA", line: &line10, column: &col15},
				{url: "fileB.js", funcName: "funcB", line: &line20, column: &col25},
			},
			expectedMode: "stack",
		},
		{
			name:          "Stack trace without exception info uses empty strings",
			exceptionName: "",
			exceptionMsg:  "",
			stack: "" +
				"Error: Something went wrong\n" + // stack[0]
				"    at funcA (fileA.js:10:15)\n" + // stack[1]
				"    at funcB (fileB.js:20:25)", // stack[2]
			expectedName:    "",
			expectedMessage: "",
			expectedFrames: []stackFrame{
				{url: "fileA.js", funcName: "funcA", line: &line10, column: &col15},
				{url: "fileB.js", funcName: "funcB", line: &line20, column: &col25},
			},
			expectedMode: "stack",
		},
		{
			name:          "Stack trace with 'native' frames",
			exceptionName: "Error",
			exceptionMsg:  "Test error",
			stack: "" +
				"Error: Test error\n" +
				"   at Array.map (native)\n" + // stack[0]
				"   at funcA (fileA.js:10:15)\n" + // stack[1]
				"   at Array.forEach (native)\n" + // stack[2]
				"   at funcB (fileB.js:20:25)", // stack[3]
			expectedName:    "Error",
			expectedMessage: "Test error",
			expectedFrames: []stackFrame{
				{url: "", funcName: "Array.map", line: nil, column: nil},
				{url: "fileA.js", funcName: "funcA", line: &line10, column: &col15},
				{url: "", funcName: "Array.forEach", line: nil, column: nil},
				{url: "fileB.js", funcName: "funcB", line: &line20, column: &col25},
			},
			expectedMode: "stack",
		},
		{
			name:          "Stack trace with anonymous functions",
			exceptionName: "Error",
			exceptionMsg:  "",
			stack: "" +
				"  Error: \n" +
				"    at new <anonymous> (http://example.com/js/test.js:63:1)\n" + // stack[0]
				"    at namedFunc0 (http://example.com/js/script.js:10:2)\n" + // stack[1]
				"    at http://example.com/js/test.js:65:10\n" + // stack[2]
				"    at namedFunc2 (http://example.com/js/script.js:20:5)\n" + // stack[3]
				"    at http://example.com/js/test.js:67:5\n" + // stack[4]
				"    at namedFunc4 (http://example.com/js/script.js:100001:10002)", // stack[5]
			expectedFrames: []stackFrame{
				{url: "http://example.com/js/test.js", funcName: "new <anonymous>", line: intPtr(63), column: intPtr(1)},
				{url: "http://example.com/js/script.js", funcName: "namedFunc0", line: intPtr(10), column: intPtr(2)},
				{url: "http://example.com/js/test.js", funcName: unknownFunction, line: intPtr(65), column: intPtr(10)},
				{url: "http://example.com/js/script.js", funcName: "namedFunc2", line: intPtr(20), column: intPtr(5)},
				{url: "http://example.com/js/test.js", funcName: unknownFunction, line: intPtr(67), column: intPtr(5)},
				{url: "http://example.com/js/script.js", funcName: "namedFunc4", line: intPtr(100001), column: intPtr(10002)},
			},
			expectedName:    "Error",
			expectedMessage: "",
			expectedMode:    "stack",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := computeStackTrace(tt.exceptionName, tt.exceptionMsg, tt.stack)

			require.NotNil(t, result)
			assert.Equal(t, tt.expectedName, result.name)
			assert.Equal(t, tt.expectedMessage, result.message)
			assert.Equal(t, tt.expectedMode, result.mode)
			assert.Equal(t, len(tt.expectedFrames), len(result.stackFrames))

			for i, expectedFrame := range tt.expectedFrames {
				assert.Equal(t, expectedFrame.url, result.stackFrames[i].url)
				assert.Equal(t, expectedFrame.funcName, result.stackFrames[i].funcName)
				if expectedFrame.line != nil {
					require.NotNil(t, result.stackFrames[i].line)
					assert.Equal(t, *expectedFrame.line, *result.stackFrames[i].line)
				} else {
					assert.Nil(t, result.stackFrames[i].line)
				}
				if expectedFrame.column != nil {
					require.NotNil(t, result.stackFrames[i].column)
					assert.Equal(t, *expectedFrame.column, *result.stackFrames[i].column)
				} else {
					assert.Nil(t, result.stackFrames[i].column)
				}
			}
		})
	}
}
