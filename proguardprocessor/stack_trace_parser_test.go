package proguardprocessor

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseStackTrace(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		expectedType     string
		expectedMessage  string
		expectedElements []element
		expectError      error
	}{
		{
			name: "Standard Java stack trace",
			input: `java.lang.RuntimeException: Something went wrong
	at com.example.MyClass.myMethod(MyClass.java:123)
	at com.example.AnotherClass.anotherMethod(AnotherClass.java:456)`,
			expectedType:    "java.lang.RuntimeException",
			expectedMessage: "Something went wrong",
			expectedElements: []element{
				{frame: &stackFrame{class: "com.example.MyClass", method: "myMethod", sourceFile: "MyClass.java", line: 123}},
				{frame: &stackFrame{class: "com.example.AnotherClass", method: "anotherMethod", sourceFile: "AnotherClass.java", line: 456}},
			},
		},
		{
			name: "Stack trace with Native Method",
			input: `java.lang.NullPointerException: Null value
	at com.example.MyClass.method1(MyClass.java:100)
	at com.example.NativeClass.nativeMethod(Native Method)
	at com.example.MyClass.method2(MyClass.java:200)`,
			expectedType:    "java.lang.NullPointerException",
			expectedMessage: "Null value",
			expectedElements: []element{
				{frame: &stackFrame{class: "com.example.MyClass", method: "method1", sourceFile: "MyClass.java", line: 100}},
				{frame: &stackFrame{class: "com.example.NativeClass", method: "nativeMethod", sourceFile: "Native Method", line: -2}},
				{frame: &stackFrame{class: "com.example.MyClass", method: "method2", sourceFile: "MyClass.java", line: 200}},
			},
		},
		{
			name: "Stack trace with Unknown Source",
			input: `java.io.IOException: IO error
	at com.example.MyClass.method1(MyClass.java:50)
	at com.example.UnknownClass.unknownMethod(Unknown Source)`,
			expectedType:    "java.io.IOException",
			expectedMessage: "IO error",
			expectedElements: []element{
				{frame: &stackFrame{class: "com.example.MyClass", method: "method1", sourceFile: "MyClass.java", line: 50}},
				{frame: &stackFrame{class: "com.example.UnknownClass", method: "unknownMethod", sourceFile: "Unknown Source", line: -1}},
			},
		},
		{
			name: "Stack trace with no line numbers",
			input: `java.lang.Exception: Test
	at com.example.MyClass.method(MyClass.java)
	at com.example.AnotherClass.method(AnotherClass.java:100)`,
			expectedType:    "java.lang.Exception",
			expectedMessage: "Test",
			expectedElements: []element{
				{frame: &stackFrame{class: "com.example.MyClass", method: "method", sourceFile: "MyClass.java", line: -1}},
				{frame: &stackFrame{class: "com.example.AnotherClass", method: "method", sourceFile: "AnotherClass.java", line: 100}},
			},
		},
		{
			name: "Stack trace with explicit negative line numbers",
			input: `java.lang.RuntimeException: Error
	at com.example.MyClass.method(MyClass.java:-1)
	at com.example.AnotherClass.method(AnotherClass.java:-2)`,
			expectedType:    "java.lang.RuntimeException",
			expectedMessage: "Error",
			expectedElements: []element{
				{frame: &stackFrame{class: "com.example.MyClass", method: "method", sourceFile: "MyClass.java", line: -1}},
				{frame: &stackFrame{class: "com.example.AnotherClass", method: "method", sourceFile: "AnotherClass.java", line: -2}},
			},
		},
		{
			name: "Obfuscated stack trace",
			input: `java.lang.RuntimeException: Error
	at a.b.c.d(SourceFile:10)
	at x.y.z(SourceFile:20)`,
			expectedType:    "java.lang.RuntimeException",
			expectedMessage: "Error",
			expectedElements: []element{
				{frame: &stackFrame{class: "a.b.c", method: "d", sourceFile: "SourceFile", line: 10}},
				{frame: &stackFrame{class: "x.y", method: "z", sourceFile: "SourceFile", line: 20}},
			},
		},
		{
			name: "Inner class stack trace",
			input: `java.lang.IllegalStateException: Bad state
	at com.example.OuterClass$InnerClass.method(OuterClass.java:100)`,
			expectedType:    "java.lang.IllegalStateException",
			expectedMessage: "Bad state",
			expectedElements: []element{
				{frame: &stackFrame{class: "com.example.OuterClass$InnerClass", method: "method", sourceFile: "OuterClass.java", line: 100}},
			},
		},
		{
			name: "Stack trace with Caused by (should preserve all lines)",
			input: `java.lang.RuntimeException: Error
	at com.example.MyClass.method(MyClass.java:100)
Caused by: java.lang.IOException: IO error
	at com.example.IOClass.read(IOClass.java:50)`,
			expectedType:    "java.lang.RuntimeException",
			expectedMessage: "Error",
			expectedElements: []element{
				{frame: &stackFrame{class: "com.example.MyClass", method: "method", sourceFile: "MyClass.java", line: 100}},
				{line: "Caused by: java.lang.IOException: IO error"},
				{frame: &stackFrame{class: "com.example.IOClass", method: "read", sourceFile: "IOClass.java", line: 50}},
			},
		},
		{
			name: "Stack trace with empty lines",
			input: `java.lang.RuntimeException: Error
	at com.example.MyClass.method(MyClass.java:100)

	at com.example.AnotherClass.method(AnotherClass.java:200)`,
			expectedType:    "java.lang.RuntimeException",
			expectedMessage: "Error",
			expectedElements: []element{
				{frame: &stackFrame{class: "com.example.MyClass", method: "method", sourceFile: "MyClass.java", line: 100}},
				{frame: &stackFrame{class: "com.example.AnotherClass", method: "method", sourceFile: "AnotherClass.java", line: 200}},
			},
		},
		{
			name: "Exception with empty message",
			input: `java.lang.RuntimeException:
	at com.example.MyClass.method(MyClass.java:100)`,
			expectedType:    "java.lang.RuntimeException",
			expectedMessage: "",
			expectedElements: []element{
				{frame: &stackFrame{class: "com.example.MyClass", method: "method", sourceFile: "MyClass.java", line: 100}},
			},
		},
		{
			name: "Exception header with extra colons",
			input: `foo: bar: baz
	at com.example.MyClass.method(MyClass.java:100)`,
			expectedType:    "foo",
			expectedMessage: "bar: baz",
			expectedElements: []element{
				{frame: &stackFrame{class: "com.example.MyClass", method: "method", sourceFile: "MyClass.java", line: 100}},
			},
		},
		{
			name: "Exception header with whitespace around colon",
			input: `foo.bar  :  baz
	at com.example.MyClass.method(MyClass.java:100)`,
			expectedType:    "foo.bar",
			expectedMessage: "baz",
			expectedElements: []element{
				{frame: &stackFrame{class: "com.example.MyClass", method: "method", sourceFile: "MyClass.java", line: 100}},
			},
		},
		{
			name: "Exception header containing exception type with no .",
			input: `Foo: bar baz
	at com.example.MyClass.method(MyClass.java:100)`,
			expectedType:    "Foo",
			expectedMessage: "bar baz",
			expectedElements: []element{
				{frame: &stackFrame{class: "com.example.MyClass", method: "method", sourceFile: "MyClass.java", line: 100}},
			},
		},
		{
			name:        "Empty string",
			input:       "",
			expectError: errEmptyStackTrace,
		},
		{
			name:        "Only exception header, no frames",
			input:       "java.lang.RuntimeException: Error",
			expectError: errNoFramesParsed,
		},
		{
			name:        "No exception header",
			input:       "\tat com.example.MyClass.method(MyClass.java:100)",
			expectError: errInvalidStackTrace,
		},
		{
			name:        "Invalid format - just random text",
			input:       "This is not a stack trace",
			expectError: errInvalidStackTrace,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseStackTrace(tt.input)

			if tt.expectError != nil {
				assert.ErrorIs(t, err, tt.expectError)
				assert.Nil(t, result)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.expectedType, result.exceptionType)
			assert.Equal(t, tt.expectedMessage, result.exceptionMessage)

			// Check expected elements if provided
			if tt.expectedElements != nil {
				assert.Equal(t, tt.expectedElements, result.elements)
			}
		})
	}
}

func TestParseStackFrame(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected *stackFrame
	}{
		{
			name:  "Standard frame",
			input: "\tat com.example.MyClass.myMethod(MyClass.java:123)",
			expected: &stackFrame{
				class:      "com.example.MyClass",
				method:     "myMethod",
				sourceFile: "MyClass.java",
				line:       123,
			},
		},
		{
			name:  "Native Method",
			input: "\tat com.example.NativeClass.nativeMethod(Native Method)",
			expected: &stackFrame{
				class:      "com.example.NativeClass",
				method:     "nativeMethod",
				sourceFile: "Native Method",
				line:       -2,
			},
		},
		{
			name:  "Unknown Source",
			input: "\tat com.example.UnknownClass.unknownMethod(Unknown Source)",
			expected: &stackFrame{
				class:      "com.example.UnknownClass",
				method:     "unknownMethod",
				sourceFile: "Unknown Source",
				line:       -1,
			},
		},
		{
			name:  "No line number",
			input: "\tat com.example.MyClass.method(MyClass.java)",
			expected: &stackFrame{
				class:      "com.example.MyClass",
				method:     "method",
				sourceFile: "MyClass.java",
				line:       -1,
			},
		},
		{
			name:  "Explicit negative line number -1",
			input: "\tat com.example.MyClass.method(MyClass.java:-1)",
			expected: &stackFrame{
				class:      "com.example.MyClass",
				method:     "method",
				sourceFile: "MyClass.java",
				line:       -1,
			},
		},
		{
			name:  "Explicit negative line number -2",
			input: "\tat com.example.MyClass.method(MyClass.java:-2)",
			expected: &stackFrame{
				class:      "com.example.MyClass",
				method:     "method",
				sourceFile: "MyClass.java",
				line:       -2,
			},
		},
		{
			name:  "Obfuscated names",
			input: "\tat a.b.c(SourceFile:10)",
			expected: &stackFrame{
				class:      "a.b",
				method:     "c",
				sourceFile: "SourceFile",
				line:       10,
			},
		},
		{
			name:  "Single character obfuscated",
			input: "\tat a.b(SourceFile:5)",
			expected: &stackFrame{
				class:      "a",
				method:     "b",
				sourceFile: "SourceFile",
				line:       5,
			},
		},
		{
			name:  "Inner class",
			input: "\tat com.example.Outer$Inner.method(Outer.java:50)",
			expected: &stackFrame{
				class:      "com.example.Outer$Inner",
				method:     "method",
				sourceFile: "Outer.java",
				line:       50,
			},
		},
		{
			name:  "Anonymous inner class",
			input: "\tat com.example.MyClass$1.run(MyClass.java:30)",
			expected: &stackFrame{
				class:      "com.example.MyClass$1",
				method:     "run",
				sourceFile: "MyClass.java",
				line:       30,
			},
		},
		{
			name:     "Invalid line - not a stack frame",
			input:    "This is not a stack frame",
			expected: nil,
		},
		{
			name:     "Empty line",
			input:    "",
			expected: nil,
		},
		{
			name:     "Missing 'at' keyword",
			input:    "com.example.MyClass.method(MyClass.java:100)",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseStackFrame(tt.input)
			if tt.expected == nil {
				assert.Nil(t, result)
			} else {
				require.NotNil(t, result)
				assert.Equal(t, *tt.expected, *result)
			}
		})
	}
}
