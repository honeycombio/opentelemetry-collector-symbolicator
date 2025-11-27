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
		expectedFrames  []StackFrame
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
			expectedFrames: []StackFrame{
				{URL: "fileA.js", Func: "funcA", Line: &line10, Column: &col15},
				{URL: "fileB.js", Func: "funcB", Line: &line20, Column: &col25},
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
			expectedFrames: []StackFrame{
				{URL: "fileA.js", Func: "funcA", Line: &line10, Column: &col15},
				{URL: "fileB.js", Func: "funcB", Line: &line20, Column: &col25},
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
			expectedFrames: []StackFrame{
				{URL: "", Func: "Array.map", Line: nil, Column: nil},
				{URL: "fileA.js", Func: "funcA", Line: &line10, Column: &col15},
				{URL: "", Func: "Array.forEach", Line: nil, Column: nil},
				{URL: "fileB.js", Func: "funcB", Line: &line20, Column: &col25},
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
			expectedFrames: []StackFrame{
				{URL: "http://example.com/js/test.js", Func: "new <anonymous>", Line: intPtr(63), Column: intPtr(1)},
				{URL: "http://example.com/js/script.js", Func: "namedFunc0", Line: intPtr(10), Column: intPtr(2)},
				{URL: "http://example.com/js/test.js", Func: UnknownFunction, Line: intPtr(65), Column: intPtr(10)},
				{URL: "http://example.com/js/script.js", Func: "namedFunc2", Line: intPtr(20), Column: intPtr(5)},
				{URL: "http://example.com/js/test.js", Func: UnknownFunction, Line: intPtr(67), Column: intPtr(5)},
				{URL: "http://example.com/js/script.js", Func: "namedFunc4", Line: intPtr(100001), Column: intPtr(10002)},
			},
			expectedName:    "Error",
			expectedMessage: "",
			expectedMode:    "stack",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tk := NewTraceKit()
			result := tk.ComputeStackTrace(tt.exceptionName, tt.exceptionMsg, tt.stack)

			require.NotNil(t, result)
			assert.Equal(t, tt.expectedName, result.Name)
			assert.Equal(t, tt.expectedMessage, result.Message)
			assert.Equal(t, tt.expectedMode, result.Mode)
			assert.Equal(t, len(tt.expectedFrames), len(result.StackFrames))

			for i, expectedFrame := range tt.expectedFrames {
				assert.Equal(t, expectedFrame.URL, result.StackFrames[i].URL)
				assert.Equal(t, expectedFrame.Func, result.StackFrames[i].Func)
				if expectedFrame.Line != nil {
					require.NotNil(t, result.StackFrames[i].Line)
					assert.Equal(t, *expectedFrame.Line, *result.StackFrames[i].Line)
				} else {
					assert.Nil(t, result.StackFrames[i].Line)
				}
				if expectedFrame.Column != nil {
					require.NotNil(t, result.StackFrames[i].Column)
					assert.Equal(t, *expectedFrame.Column, *result.StackFrames[i].Column)
				} else {
					assert.Nil(t, result.StackFrames[i].Column)
				}
			}
		})
	}
}
