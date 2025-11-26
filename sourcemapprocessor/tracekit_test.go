package sourcemapprocessor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
			name:            "Valid TraceKit stack trace with provided exception info",
			exceptionName:   "Error",
			exceptionMsg:    "Something went wrong",
			stack:           "Error: Something went wrong\n    at funcA (fileA.js:10:15)\n    at funcB (fileB.js:20:25)",
			expectedName:    "Error",
			expectedMessage: "Something went wrong",
			expectedFrames: []StackFrame{
				{URL: "fileA.js", Func: "funcA", Args: []string{}, Line: &line10, Column: &col15, Context: nil},
				{URL: "fileB.js", Func: "funcB", Args: []string{}, Line: &line20, Column: &col25, Context: nil},
			},
			expectedMode: "stack",
		},
		{
			name:            "Stack trace without exception info uses empty strings",
			exceptionName:   "",
			exceptionMsg:    "",
			stack:           "Error: Something went wrong\n    at funcA (fileA.js:10:15)\n    at funcB (fileB.js:20:25)",
			expectedName:    "",
			expectedMessage: "",
			expectedFrames: []StackFrame{
				{URL: "fileA.js", Func: "funcA", Args: []string{}, Line: &line10, Column: &col15, Context: nil},
				{URL: "fileB.js", Func: "funcB", Args: []string{}, Line: &line20, Column: &col25, Context: nil},
			},
			expectedMode: "stack",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tk := NewTraceKit()
			result := tk.ComputeStackTrace(tt.exceptionName, tt.exceptionMsg, tt.stack, 0)

			require.NotNil(t, result)
			assert.Equal(t, tt.expectedName, result.Name)
			assert.Equal(t, tt.expectedMessage, result.Message)
			assert.Equal(t, tt.expectedMode, result.Mode)
			assert.Equal(t, len(tt.expectedFrames), len(result.Stack))

			for i, expectedFrame := range tt.expectedFrames {
				assert.Equal(t, expectedFrame.URL, result.Stack[i].URL)
				assert.Equal(t, expectedFrame.Func, result.Stack[i].Func)
				assert.Equal(t, expectedFrame.Args, result.Stack[i].Args)
				if expectedFrame.Line != nil {
					require.NotNil(t, result.Stack[i].Line)
					assert.Equal(t, *expectedFrame.Line, *result.Stack[i].Line)
				}
				if expectedFrame.Column != nil {
					require.NotNil(t, result.Stack[i].Column)
					assert.Equal(t, *expectedFrame.Column, *result.Stack[i].Column)
				}
			}
		})
	}
}