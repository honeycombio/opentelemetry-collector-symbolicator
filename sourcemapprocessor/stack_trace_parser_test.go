// Some unit tests are adapted from TraceKit, a JavaScript error tracing and
// stack trace parsing library.
// TraceKit is MIT-licensed and available at: https://github.com/csnover/TraceKit

package sourcemapprocessor

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func intPtr(i int) *int {
	return &i
}

func TestStackTraceParser(t *testing.T) {
	tests := []struct {
		name            string
		exceptionName   string
		exceptionMsg    string
		stack           string
		expectedName    string
		expectedMessage string
		expectedFrames  []stackFrame
		expectedMode    parseMode
	}{
		// Safari Tests
		{
			name:          "Safari 6 error",
			exceptionName: "TypeError",
			exceptionMsg:  "'null' is not an object (evaluating 'x.undef')",
			stack: "@http://path/to/file.js:48\n" +
				"dumpException3@http://path/to/file.js:52\n" +
				"onclick@http://path/to/file.js:82\n" +
				"[native code]",
			expectedName:    "TypeError",
			expectedMessage: "'null' is not an object (evaluating 'x.undef')",
			expectedFrames: []stackFrame{
				{url: "http://path/to/file.js", funcName: unknownFunction, line: intPtr(48), column: nil},
				{url: "http://path/to/file.js", funcName: "dumpException3", line: intPtr(52), column: nil},
				{url: "http://path/to/file.js", funcName: "onclick", line: intPtr(82), column: nil},
				{url: "[native code]", funcName: unknownFunction, line: nil, column: nil},
			},
			expectedMode: parseModeStack,
		},
		{
			name:          "Safari 7 error",
			exceptionName: "TypeError",
			exceptionMsg:  "'null' is not an object (evaluating 'x.undef')",
			stack: "http://path/to/file.js:48:22\n" +
				"foo@http://path/to/file.js:52:15\n" +
				"bar@http://path/to/file.js:108:107",
			expectedName:    "TypeError",
			expectedMessage: "'null' is not an object (evaluating 'x.undef')",
			expectedFrames: []stackFrame{
				{url: "http://path/to/file.js", funcName: unknownFunction, line: intPtr(48), column: intPtr(22)},
				{url: "http://path/to/file.js", funcName: "foo", line: intPtr(52), column: intPtr(15)},
				{url: "http://path/to/file.js", funcName: "bar", line: intPtr(108), column: intPtr(107)},
			},
			expectedMode: parseModeStack,
		},
		{
			name:          "Safari 8 error",
			exceptionName: "TypeError",
			exceptionMsg:  "null is not an object (evaluating 'x.undef')",
			stack: "http://path/to/file.js:47:22\n" +
				"foo@http://path/to/file.js:52:15\n" +
				"bar@http://path/to/file.js:108:23",
			expectedName:    "TypeError",
			expectedMessage: "null is not an object (evaluating 'x.undef')",
			expectedFrames: []stackFrame{
				{url: "http://path/to/file.js", funcName: unknownFunction, line: intPtr(47), column: intPtr(22)},
				{url: "http://path/to/file.js", funcName: "foo", line: intPtr(52), column: intPtr(15)},
				{url: "http://path/to/file.js", funcName: "bar", line: intPtr(108), column: intPtr(23)},
			},
			expectedMode: parseModeStack,
		},
		{
			name:          "Safari 8 eval error",
			exceptionName: "ReferenceError",
			exceptionMsg:  "Can't find variable: getExceptionProps",
			stack: "eval code\n" +
				"eval@[native code]\n" +
				"foo@http://path/to/file.js:58:21\n" +
				"bar@http://path/to/file.js:109:91",
			expectedName:    "ReferenceError",
			expectedMessage: "Can't find variable: getExceptionProps",
			expectedFrames: []stackFrame{
				{url: "[native code]", funcName: "eval", line: nil, column: nil},
				{url: "http://path/to/file.js", funcName: "foo", line: intPtr(58), column: intPtr(21)},
				{url: "http://path/to/file.js", funcName: "bar", line: intPtr(109), column: intPtr(91)},
			},
			expectedMode: parseModeStack,
		},

		// Firefox Tests
		{
			name:          "Firefox 3 error",
			exceptionName: "TypeError",
			exceptionMsg:  "this.undef is not a function",
			stack: "()@http://127.0.0.1:8000/js/stacktrace.js:44\n" +
				"(null)@http://127.0.0.1:8000/js/stacktrace.js:31\n" +
				"printStackTrace()@http://127.0.0.1:8000/js/stacktrace.js:18\n" +
				"bar(1)@http://127.0.0.1:8000/js/file.js:13\n" +
				"bar(2)@http://127.0.0.1:8000/js/file.js:16\n" +
				"foo()@http://127.0.0.1:8000/js/file.js:20\n" +
				"@http://127.0.0.1:8000/js/file.js:24\n",
			expectedName:    "TypeError",
			expectedMessage: "this.undef is not a function",
			expectedFrames: []stackFrame{
				{url: "http://127.0.0.1:8000/js/stacktrace.js", funcName: unknownFunction, line: intPtr(44), column: nil},
				{url: "http://127.0.0.1:8000/js/stacktrace.js", funcName: unknownFunction, line: intPtr(31), column: nil},
				{url: "http://127.0.0.1:8000/js/stacktrace.js", funcName: "printStackTrace", line: intPtr(18), column: nil},
				{url: "http://127.0.0.1:8000/js/file.js", funcName: "bar", line: intPtr(13), column: nil},
				{url: "http://127.0.0.1:8000/js/file.js", funcName: "bar", line: intPtr(16), column: nil},
				{url: "http://127.0.0.1:8000/js/file.js", funcName: "foo", line: intPtr(20), column: nil},
				{url: "http://127.0.0.1:8000/js/file.js", funcName: unknownFunction, line: intPtr(24), column: nil},
			},
			expectedMode: parseModeStack,
		},
		{
			name:          "Firefox 7 error",
			exceptionName: "TypeError",
			exceptionMsg:  "this.undef is not a function",
			stack: "()@file:///G:/js/stacktrace.js:44\n" +
				"(null)@file:///G:/js/stacktrace.js:31\n" +
				"printStackTrace()@file:///G:/js/stacktrace.js:18\n" +
				"bar(1)@file:///G:/js/file.js:13\n" +
				"bar(2)@file:///G:/js/file.js:16\n" +
				"foo()@file:///G:/js/file.js:20\n" +
				"@file:///G:/js/file.js:24\n",
			expectedName:    "TypeError",
			expectedMessage: "this.undef is not a function",
			expectedFrames: []stackFrame{
				{url: "file:///G:/js/stacktrace.js", funcName: unknownFunction, line: intPtr(44), column: nil},
				{url: "file:///G:/js/stacktrace.js", funcName: unknownFunction, line: intPtr(31), column: nil},
				{url: "file:///G:/js/stacktrace.js", funcName: "printStackTrace", line: intPtr(18), column: nil},
				{url: "file:///G:/js/file.js", funcName: "bar", line: intPtr(13), column: nil},
				{url: "file:///G:/js/file.js", funcName: "bar", line: intPtr(16), column: nil},
				{url: "file:///G:/js/file.js", funcName: "foo", line: intPtr(20), column: nil},
				{url: "file:///G:/js/file.js", funcName: unknownFunction, line: intPtr(24), column: nil},
			},
			expectedMode: parseModeStack,
		},
		{
			name:          "Firefox 14 error",
			exceptionName: "TypeError",
			exceptionMsg:  "x is null",
			stack: "@http://path/to/file.js:48\n" +
				"dumpException3@http://path/to/file.js:52\n" +
				"onclick@http://path/to/file.js:1\n",
			expectedName:    "TypeError",
			expectedMessage: "x is null",
			expectedFrames: []stackFrame{
				{url: "http://path/to/file.js", funcName: unknownFunction, line: intPtr(48), column: nil},
				{url: "http://path/to/file.js", funcName: "dumpException3", line: intPtr(52), column: nil},
				{url: "http://path/to/file.js", funcName: "onclick", line: intPtr(1), column: nil},
			},
			expectedMode: parseModeStack,
		},
		{
			name:          "Firefox 31 error",
			exceptionName: "Error",
			exceptionMsg:  "Default error",
			stack: "foo@http://path/to/file.js:41:13\n" +
				"bar@http://path/to/file.js:1:1\n" +
				".plugin/e.fn[c]/<@http://path/to/file.js:1:1\n",
			expectedName:    "Error",
			expectedMessage: "Default error",
			expectedFrames: []stackFrame{
				{url: "http://path/to/file.js", funcName: "foo", line: intPtr(41), column: intPtr(13)},
				{url: "http://path/to/file.js", funcName: "bar", line: intPtr(1), column: intPtr(1)},
				{url: "http://path/to/file.js", funcName: ".plugin/e.fn[c]/<", line: intPtr(1), column: intPtr(1)},
			},
			expectedMode: parseModeStack,
		},
		{
			name:          "Firefox 43 eval error",
			exceptionName: "Error",
			exceptionMsg:  "message string",
			stack: "baz@http://localhost:8080/file.js line 26 > eval line 2 > eval:1:30\n" +
				"foo@http://localhost:8080/file.js line 26 > eval:2:96\n" +
				"@http://localhost:8080/file.js line 26 > eval:4:18\n" +
				"speak@http://localhost:8080/file.js:26:17\n" +
				"@http://localhost:8080/file.js:33:9",
			expectedName:    "Error",
			expectedMessage: "message string",
			expectedFrames: []stackFrame{
				{url: "http://localhost:8080/file.js", funcName: "baz", line: intPtr(26), column: nil},
				{url: "http://localhost:8080/file.js", funcName: "foo", line: intPtr(26), column: nil},
				{url: "http://localhost:8080/file.js", funcName: unknownFunction, line: intPtr(26), column: nil},
				{url: "http://localhost:8080/file.js", funcName: "speak", line: intPtr(26), column: intPtr(17)},
				{url: "http://localhost:8080/file.js", funcName: unknownFunction, line: intPtr(33), column: intPtr(9)},
			},
			expectedMode: parseModeStack,
		},
		{
			name:          "Firefox 44 NS Exception",
			exceptionName: "NS_ERROR_FAILURE",
			exceptionMsg:  "",
			stack: "[2]</Bar.prototype._baz/</<@http://path/to/file.js:703:28\n" +
				"App.prototype.foo@file:///path/to/file.js:15:2\n" +
				"bar@file:///path/to/file.js:20:3\n" +
				"@file:///path/to/index.html:23:1\n",
			expectedName:    "NS_ERROR_FAILURE",
			expectedMessage: "",
			expectedFrames: []stackFrame{
				{url: "http://path/to/file.js", funcName: "[2]</Bar.prototype._baz/</<", line: intPtr(703), column: intPtr(28)},
				{url: "file:///path/to/file.js", funcName: "App.prototype.foo", line: intPtr(15), column: intPtr(2)},
				{url: "file:///path/to/file.js", funcName: "bar", line: intPtr(20), column: intPtr(3)},
				{url: "file:///path/to/index.html", funcName: unknownFunction, line: intPtr(23), column: intPtr(1)},
			},
			expectedMode: parseModeStack,
		},
		{
			name:          "Firefox 50 resource URL",
			exceptionName: "TypeError",
			exceptionMsg:  "this.props.raw[this.state.dataSource].rows is undefined",
			stack: "render@resource://path/data/content/bundle.js:5529:16\n" +
				"dispatchEvent@resource://path/data/content/vendor.bundle.js:18:23028\n" +
				"wrapped@resource://path/data/content/bundle.js:7270:25",
			expectedName:    "TypeError",
			expectedMessage: "this.props.raw[this.state.dataSource].rows is undefined",
			expectedFrames: []stackFrame{
				{url: "resource://path/data/content/bundle.js", funcName: "render", line: intPtr(5529), column: intPtr(16)},
				{url: "resource://path/data/content/vendor.bundle.js", funcName: "dispatchEvent", line: intPtr(18), column: intPtr(23028)},
				{url: "resource://path/data/content/bundle.js", funcName: "wrapped", line: intPtr(7270), column: intPtr(25)},
			},
			expectedMode: parseModeStack,
		},

		// Chrome Tests
		{
			name:          "Chrome 15 error",
			exceptionName: "TypeError",
			exceptionMsg:  "Object #<Object> has no method 'undef'",
			stack: "TypeError: Object #<Object> has no method 'undef'\n" +
				"    at bar (http://path/to/file.js:13:17)\n" +
				"    at bar (http://path/to/file.js:16:5)\n" +
				"    at foo (http://path/to/file.js:20:5)\n" +
				"    at http://path/to/file.js:24:4",
			expectedName:    "TypeError",
			expectedMessage: "Object #<Object> has no method 'undef'",
			expectedFrames: []stackFrame{
				{url: "http://path/to/file.js", funcName: "bar", line: intPtr(13), column: intPtr(17)},
				{url: "http://path/to/file.js", funcName: "bar", line: intPtr(16), column: intPtr(5)},
				{url: "http://path/to/file.js", funcName: "foo", line: intPtr(20), column: intPtr(5)},
				{url: "http://path/to/file.js", funcName: unknownFunction, line: intPtr(24), column: intPtr(4)},
			},
			expectedMode: parseModeStack,
		},
		{
			name:          "Chrome 36 error with port numbers",
			exceptionName: "Error",
			exceptionMsg:  "Default error",
			stack: "Error: Default error\n" +
				"    at dumpExceptionError (http://localhost:8080/file.js:41:27)\n" +
				"    at HTMLButtonElement.onclick (http://localhost:8080/file.js:107:146)\n" +
				"    at I.e.fn.(anonymous function) [as index] (http://localhost:8080/file.js:10:3651)",
			expectedName:    "Error",
			expectedMessage: "Default error",
			expectedFrames: []stackFrame{
				{url: "http://localhost:8080/file.js", funcName: "dumpExceptionError", line: intPtr(41), column: intPtr(27)},
				{url: "http://localhost:8080/file.js", funcName: "HTMLButtonElement.onclick", line: intPtr(107), column: intPtr(146)},
				{url: "http://localhost:8080/file.js", funcName: "I.e.fn.(anonymous function) [as index]", line: intPtr(10), column: intPtr(3651)},
			},
			expectedMode: parseModeStack,
		},
		{
			name:          "Chrome error with webpack URLs",
			exceptionName: "TypeError",
			exceptionMsg:  "Cannot read property 'error' of undefined",
			stack: "TypeError: Cannot read property 'error' of undefined\n" +
				"   at TESTTESTTEST.eval(webpack:///./src/components/test/test.jsx?:295:108)\n" +
				"   at TESTTESTTEST.render(webpack:///./src/components/test/test.jsx?:272:32)\n" +
				"   at TESTTESTTEST.tryRender(webpack:///./~/react-transform-catch-errors/lib/index.js?:34:31)\n" +
				"   at TESTTESTTEST.proxiedMethod(webpack:///./~/react-proxy/modules/createPrototypeProxy.js?:44:30)",
			expectedName:    "TypeError",
			expectedMessage: "Cannot read property 'error' of undefined",
			expectedFrames: []stackFrame{
				{url: "webpack:///./src/components/test/test.jsx?", funcName: "TESTTESTTEST.eval", line: intPtr(295), column: intPtr(108)},
				{url: "webpack:///./src/components/test/test.jsx?", funcName: "TESTTESTTEST.render", line: intPtr(272), column: intPtr(32)},
				{url: "webpack:///./~/react-transform-catch-errors/lib/index.js?", funcName: "TESTTESTTEST.tryRender", line: intPtr(34), column: intPtr(31)},
				{url: "webpack:///./~/react-proxy/modules/createPrototypeProxy.js?", funcName: "TESTTESTTEST.proxiedMethod", line: intPtr(44), column: intPtr(30)},
			},
			expectedMode: parseModeStack,
		},
		{
			name:          "Chrome 48 nested eval",
			exceptionName: "Error",
			exceptionMsg:  "message string",
			stack: "Error: message string\n" +
				"at baz (eval at foo (eval at speak (http://localhost:8080/file.js:21:17)), <anonymous>:1:30)\n" +
				"at foo (eval at speak (http://localhost:8080/file.js:21:17), <anonymous>:2:96)\n" +
				"at eval (eval at speak (http://localhost:8080/file.js:21:17), <anonymous>:4:18)\n" +
				"at Object.speak (http://localhost:8080/file.js:21:17)\n" +
				"at http://localhost:8080/file.js:31:13\n",
			expectedName:    "Error",
			expectedMessage: "message string",
			expectedFrames: []stackFrame{
				{url: "http://localhost:8080/file.js", funcName: "baz", line: intPtr(21), column: intPtr(17)},
				{url: "http://localhost:8080/file.js", funcName: "foo", line: intPtr(21), column: intPtr(17)},
				{url: "http://localhost:8080/file.js", funcName: "eval", line: intPtr(21), column: intPtr(17)},
				{url: "http://localhost:8080/file.js", funcName: "Object.speak", line: intPtr(21), column: intPtr(17)},
				{url: "http://localhost:8080/file.js", funcName: unknownFunction, line: intPtr(31), column: intPtr(13)},
			},
			expectedMode: parseModeStack,
		},
		{
			name:          "Chrome 48 blob URLs",
			exceptionName: "Error",
			exceptionMsg:  "Error: test",
			stack: "Error: test\n" +
				"    at Error (native)\n" +
				"    at s (blob:http%3A//localhost%3A8080/abfc40e9-4742-44ed-9dcd-af8f99a29379:31:29146)\n" +
				"    at Object.d [as add] (blob:http%3A//localhost%3A8080/abfc40e9-4742-44ed-9dcd-af8f99a29379:31:30039)\n" +
				"    at blob:http%3A//localhost%3A8080/d4eefe0f-361a-4682-b217-76587d9f712a:15:10978\n" +
				"    at blob:http%3A//localhost%3A8080/abfc40e9-4742-44ed-9dcd-af8f99a29379:1:6911\n" +
				"    at n.fire (blob:http%3A//localhost%3A8080/abfc40e9-4742-44ed-9dcd-af8f99a29379:7:3019)\n" +
				"    at n.handle (blob:http%3A//localhost%3A8080/abfc40e9-4742-44ed-9dcd-af8f99a29379:7:2863)",
			expectedName:    "Error",
			expectedMessage: "Error: test",
			expectedFrames: []stackFrame{
				{url: "", funcName: "Error", line: nil, column: nil},
				{url: "blob:http%3A//localhost%3A8080/abfc40e9-4742-44ed-9dcd-af8f99a29379", funcName: "s", line: intPtr(31), column: intPtr(29146)},
				{url: "blob:http%3A//localhost%3A8080/abfc40e9-4742-44ed-9dcd-af8f99a29379", funcName: "Object.d [as add]", line: intPtr(31), column: intPtr(30039)},
				{url: "blob:http%3A//localhost%3A8080/d4eefe0f-361a-4682-b217-76587d9f712a", funcName: unknownFunction, line: intPtr(15), column: intPtr(10978)},
				{url: "blob:http%3A//localhost%3A8080/abfc40e9-4742-44ed-9dcd-af8f99a29379", funcName: unknownFunction, line: intPtr(1), column: intPtr(6911)},
				{url: "blob:http%3A//localhost%3A8080/abfc40e9-4742-44ed-9dcd-af8f99a29379", funcName: "n.fire", line: intPtr(7), column: intPtr(3019)},
				{url: "blob:http%3A//localhost%3A8080/abfc40e9-4742-44ed-9dcd-af8f99a29379", funcName: "n.handle", line: intPtr(7), column: intPtr(2863)},
			},
			expectedMode: parseModeStack,
		},
		{
			name:            "Chrome error with no location (native)",
			exceptionName:   "TypeError",
			exceptionMsg:    "error",
			stack:           "error\n at Array.forEach (native)",
			expectedName:    "TypeError",
			expectedMessage: "error",
			expectedFrames: []stackFrame{
				{url: "", funcName: "Array.forEach", line: nil, column: nil},
			},
			expectedMode: parseModeStack,
		},

		// Internet Explorer Tests
		{
			name:            "IE 9 error (no stack)",
			exceptionName:   "TypeError",
			exceptionMsg:    "Unable to get property 'undef' of undefined or null reference",
			stack:           "",
			expectedName:    "TypeError",
			expectedMessage: "Unable to get property 'undef' of undefined or null reference",
			expectedFrames:  []stackFrame{},
			expectedMode:    parseModeFailed,
		},
		{
			name:          "IE 10 error",
			exceptionName: "TypeError",
			exceptionMsg:  "Unable to get property 'undef' of undefined or null reference",
			stack: "TypeError: Unable to get property 'undef' of undefined or null reference\n" +
				"   at Anonymous function (http://path/to/file.js:48:13)\n" +
				"   at foo (http://path/to/file.js:46:9)\n" +
				"   at bar (http://path/to/file.js:82:1)",
			expectedName:    "TypeError",
			expectedMessage: "Unable to get property 'undef' of undefined or null reference",
			expectedFrames: []stackFrame{
				{url: "http://path/to/file.js", funcName: "Anonymous function", line: intPtr(48), column: intPtr(13)},
				{url: "http://path/to/file.js", funcName: "foo", line: intPtr(46), column: intPtr(9)},
				{url: "http://path/to/file.js", funcName: "bar", line: intPtr(82), column: intPtr(1)},
			},
			expectedMode: parseModeStack,
		},
		{
			name:          "IE 11 error",
			exceptionName: "TypeError",
			exceptionMsg:  "Unable to get property 'undef' of undefined or null reference",
			stack: "TypeError: Unable to get property 'undef' of undefined or null reference\n" +
				"   at Anonymous function (http://path/to/file.js:47:21)\n" +
				"   at foo (http://path/to/file.js:45:13)\n" +
				"   at bar (http://path/to/file.js:108:1)",
			expectedName:    "TypeError",
			expectedMessage: "Unable to get property 'undef' of undefined or null reference",
			expectedFrames: []stackFrame{
				{url: "http://path/to/file.js", funcName: "Anonymous function", line: intPtr(47), column: intPtr(21)},
				{url: "http://path/to/file.js", funcName: "foo", line: intPtr(45), column: intPtr(13)},
				{url: "http://path/to/file.js", funcName: "bar", line: intPtr(108), column: intPtr(1)},
			},
			expectedMode: parseModeStack,
		},
		{
			name:          "IE 11 eval error",
			exceptionName: "ReferenceError",
			exceptionMsg:  "'getExceptionProps' is undefined",
			stack: "ReferenceError: 'getExceptionProps' is undefined\n" +
				"   at eval code (eval code:1:1)\n" +
				"   at foo (http://path/to/file.js:58:17)\n" +
				"   at bar (http://path/to/file.js:109:1)",
			expectedName:    "ReferenceError",
			expectedMessage: "'getExceptionProps' is undefined",
			expectedFrames: []stackFrame{
				{url: "eval code", funcName: "eval code", line: intPtr(1), column: intPtr(1)},
				{url: "http://path/to/file.js", funcName: "foo", line: intPtr(58), column: intPtr(17)},
				{url: "http://path/to/file.js", funcName: "bar", line: intPtr(109), column: intPtr(1)},
			},
			expectedMode: parseModeStack,
		},

		// Opera Tests
		{
			name:          "Opera 9.64 error with function names",
			exceptionName: "Error",
			exceptionMsg: "Statement on line 42: Type mismatch (usually non-object value supplied where object required)\n" +
				"Backtrace:\n" +
				"  Line 42 of linked script http://path/to/file.js\n" +
				"                this.undef();\n" +
				"  Line 27 of linked script http://path/to/file.js\n" +
				"            ex = ex || this.createException();\n" +
				"  Line 18 of linked script http://path/to/file.js: In function printStackTrace\n" +
				"        var p = new printStackTrace.implementation(), result = p.run(ex);\n" +
				"  Line 4 of inline#1 script in http://path/to/file.js: In function bar\n" +
				"             printTrace(printStackTrace());\n" +
				"  Line 7 of inline#1 script in http://path/to/file.js: In function bar\n" +
				"           bar(n - 1);\n" +
				"  Line 11 of inline#1 script in http://path/to/file.js: In function foo\n" +
				"           bar(2);\n" +
				"  Line 15 of inline#1 script in http://path/to/file.js\n" +
				"         foo();",
			stack:           "",
			expectedName:    "Error",
			expectedMessage: "Statement on line 42: Type mismatch (usually non-object value supplied where object required)",
			expectedFrames: []stackFrame{
				{url: "http://path/to/file.js", funcName: unknownFunction, line: intPtr(42), column: nil},
				{url: "http://path/to/file.js", funcName: unknownFunction, line: intPtr(27), column: nil},
				{url: "http://path/to/file.js", funcName: "printStackTrace", line: intPtr(18), column: nil},
				{url: "http://path/to/file.js", funcName: "bar", line: intPtr(4), column: nil},
				{url: "http://path/to/file.js", funcName: "bar", line: intPtr(7), column: nil},
				{url: "http://path/to/file.js", funcName: "foo", line: intPtr(11), column: nil},
				{url: "http://path/to/file.js", funcName: unknownFunction, line: intPtr(15), column: nil},
			},
			expectedMode: parseModeMultiline,
		},
		{
			name:          "Opera 9 error",
			exceptionName: "TypeError",
			exceptionMsg: "Statement on line 44: Type mismatch\n" +
				"Backtrace:\n" +
				"  Line 44 of linked script http://path/to/file.js\n" +
				"    this.undef();\n" +
				"  Line 31 of linked script http://path/to/file.js\n" +
				"    ex = ex || this.createException();",
			stack:           "",
			expectedName:    "TypeError",
			expectedMessage: "Statement on line 44: Type mismatch",
			expectedFrames: []stackFrame{
				{url: "http://path/to/file.js", funcName: unknownFunction, line: intPtr(44), column: nil},
				{url: "http://path/to/file.js", funcName: unknownFunction, line: intPtr(31), column: nil},
			},
			expectedMode: parseModeMultiline,
		},
		{
			name:          "Opera 25 error",
			exceptionName: "TypeError",
			exceptionMsg:  "Cannot read property 'undef' of null",
			stack: "TypeError: Cannot read property 'undef' of null\n" +
				"    at http://path/to/file.js:47:22\n" +
				"    at foo (http://path/to/file.js:52:15)\n" +
				"    at bar (http://path/to/file.js:108:168)",
			expectedName:    "TypeError",
			expectedMessage: "Cannot read property 'undef' of null",
			expectedFrames: []stackFrame{
				{url: "http://path/to/file.js", funcName: unknownFunction, line: intPtr(47), column: intPtr(22)},
				{url: "http://path/to/file.js", funcName: "foo", line: intPtr(52), column: intPtr(15)},
				{url: "http://path/to/file.js", funcName: "bar", line: intPtr(108), column: intPtr(168)},
			},
			expectedMode: parseModeStack,
		},
		{
			name:          "Opera 11 error",
			exceptionName: "Error",
			exceptionMsg:  "'this.undef' is not a function",
			stack: "Error thrown at line 42, column 12 in <anonymous function: createException>() in http://path/to/file.js:\n" +
				"    this.undef();\n" +
				"called from line 27, column 8 in <anonymous function: run>(ex) in http://path/to/file.js:\n" +
				"    ex = ex || this.createException();\n" +
				"called from line 18, column 4 in printStackTrace(options) in http://path/to/file.js:\n" +
				"    var p = new printStackTrace.implementation(), result = p.run(ex);\n" +
				"called from line 4, column 5 in bar(n) in http://path/to/file.js:\n" +
				"    printTrace(printStackTrace());\n" +
				"called from line 7, column 4 in bar(n) in http://path/to/file.js:\n" +
				"    bar(n - 1);\n" +
				"called from line 11, column 4 in foo() in http://path/to/file.js:\n" +
				"    bar(2);\n" +
				"called from line 15, column 3 in http://path/to/file.js:\n" +
				"    foo();",
			expectedName:    "Error",
			expectedMessage: "'this.undef' is not a function",
			expectedFrames: []stackFrame{
				{url: "http://path/to/file.js", funcName: "createException", line: intPtr(42), column: intPtr(12)},
				{url: "http://path/to/file.js", funcName: "run", line: intPtr(27), column: intPtr(8)},
				{url: "http://path/to/file.js", funcName: "printStackTrace", line: intPtr(18), column: intPtr(4)},
				{url: "http://path/to/file.js", funcName: "bar", line: intPtr(4), column: intPtr(5)},
				{url: "http://path/to/file.js", funcName: "bar", line: intPtr(7), column: intPtr(4)},
				{url: "http://path/to/file.js", funcName: "foo", line: intPtr(11), column: intPtr(4)},
				{url: "http://path/to/file.js", funcName: unknownFunction, line: intPtr(15), column: intPtr(3)},
			},
			expectedMode: parseModeStacktrace,
		},
		{
			name:          "Opera 12 error",
			exceptionName: "Error",
			exceptionMsg:  "Cannot convert 'x' to object",
			stack: "Error thrown at line 48, column 12 in <anonymous function>(x) in http://localhost:8000/ExceptionLab.html:\n" +
				"    x.undef();\n" +
				"called from line 46, column 8 in dumpException3() in http://localhost:8000/ExceptionLab.html:\n" +
				"    dumpException((function(x) {\n" +
				"called from line 1, column 0 in <anonymous function>(event) in http://localhost:8000/ExceptionLab.html:\n" +
				"    dumpException3();",
			expectedName:    "Error",
			expectedMessage: "Cannot convert 'x' to object",
			expectedFrames: []stackFrame{
				{url: "http://localhost:8000/ExceptionLab.html", funcName: "<anonymous function>", line: intPtr(48), column: intPtr(12)},
				{url: "http://localhost:8000/ExceptionLab.html", funcName: "dumpException3", line: intPtr(46), column: intPtr(8)},
				{url: "http://localhost:8000/ExceptionLab.html", funcName: "<anonymous function>", line: intPtr(1), column: intPtr(0)},
			},
			expectedMode: parseModeStacktrace,
		},
		{
			name:          "Opera 10 error",
			exceptionName: "Error",
			exceptionMsg:  "Statement on line 42: Type mismatch (usually non-object value supplied where object required)",
			stack: "  Line 42 of linked script http://path/to/file.js\n" +
				"                this.undef();\n" +
				"  Line 27 of linked script http://path/to/file.js\n" +
				"            ex = ex || this.createException();\n" +
				"  Line 18 of linked script http://path/to/file.js: In function printStackTrace\n" +
				"        var p = new printStackTrace.implementation(), result = p.run(ex);\n" +
				"  Line 4 of inline#1 script in http://path/to/file.js: In function bar\n" +
				"             printTrace(printStackTrace());\n" +
				"  Line 7 of inline#1 script in http://path/to/file.js: In function bar\n" +
				"           bar(n - 1);\n" +
				"  Line 11 of inline#1 script in http://path/to/file.js: In function foo\n" +
				"           bar(2);\n" +
				"  Line 15 of inline#1 script in http://path/to/file.js\n" +
				"         foo();\n",
			expectedName:    "Error",
			expectedMessage: "Statement on line 42: Type mismatch (usually non-object value supplied where object required)",
			expectedFrames: []stackFrame{
				{url: "http://path/to/file.js", funcName: unknownFunction, line: intPtr(42), column: nil},
				{url: "http://path/to/file.js", funcName: unknownFunction, line: intPtr(27), column: nil},
				{url: "http://path/to/file.js", funcName: "printStackTrace", line: intPtr(18), column: nil},
				{url: "http://path/to/file.js", funcName: "bar", line: intPtr(4), column: nil},
				{url: "http://path/to/file.js", funcName: "bar", line: intPtr(7), column: nil},
				{url: "http://path/to/file.js", funcName: "foo", line: intPtr(11), column: nil},
				{url: "http://path/to/file.js", funcName: unknownFunction, line: intPtr(15), column: nil},
			},
			expectedMode: parseModeStacktrace,
		},

		// PhantomJS Tests
		{
			name:          "PhantomJS 1.19 error",
			exceptionName: "Error",
			exceptionMsg:  "foo",
			stack: "Error: foo\n" +
				"    at file:///path/to/file.js:878\n" +
				"    at foo (http://path/to/file.js:4283)\n" +
				"    at http://path/to/file.js:4287",
			expectedName:    "Error",
			expectedMessage: "foo",
			expectedFrames: []stackFrame{
				{url: "file:///path/to/file.js", funcName: unknownFunction, line: intPtr(878), column: nil},
				{url: "http://path/to/file.js", funcName: "foo", line: intPtr(4283), column: nil},
				{url: "http://path/to/file.js", funcName: unknownFunction, line: intPtr(4287), column: nil},
			},
			expectedMode: parseModeStack,
		},

		// React Native Tests
		{
			name:          "Android React Native error",
			exceptionName: "Error",
			exceptionMsg:  "Error: test",
			stack: "Error: test\n" +
				"at render(/home/username/sample-workspace/sampleapp.collect.react/src/components/GpsMonitorScene.js:78:24)\n" +
				"at _renderValidatedComponentWithoutOwnerOrContext(/home/username/sample-workspace/sampleapp.collect.react/node_modules/react-native/Libraries/Renderer/src/renderers/shared/stack/reconciler/ReactCompositeComponent.js:1050:29)\n" +
				"at _renderValidatedComponent(/home/username/sample-workspace/sampleapp.collect.react/node_modules/react-native/Libraries/Renderer/src/renderers/shared/stack/reconciler/ReactCompositeComponent.js:1075:15)\n" +
				"at renderedElement(/home/username/sample-workspace/sampleapp.collect.react/node_modules/react-native/Libraries/Renderer/src/renderers/shared/stack/reconciler/ReactCompositeComponent.js:484:29)\n" +
				"at _currentElement(/home/username/sample-workspace/sampleapp.collect.react/node_modules/react-native/Libraries/Renderer/src/renderers/shared/stack/reconciler/ReactCompositeComponent.js:346:40)\n" +
				"at child(/home/username/sample-workspace/sampleapp.collect.react/node_modules/react-native/Libraries/Renderer/src/renderers/shared/stack/reconciler/ReactReconciler.js:68:25)\n" +
				"at children(/home/username/sample-workspace/sampleapp.collect.react/node_modules/react-native/Libraries/Renderer/src/renderers/shared/stack/reconciler/ReactMultiChild.js:264:10)\n" +
				"at this(/home/username/sample-workspace/sampleapp.collect.react/node_modules/react-native/Libraries/Renderer/src/renderers/native/ReactNativeBaseComponent.js:74:41)\n",
			expectedName:    "Error",
			expectedMessage: "Error: test",
			expectedFrames: []stackFrame{
				{url: "/home/username/sample-workspace/sampleapp.collect.react/src/components/GpsMonitorScene.js", funcName: "render", line: intPtr(78), column: intPtr(24)},
				{url: "/home/username/sample-workspace/sampleapp.collect.react/node_modules/react-native/Libraries/Renderer/src/renderers/shared/stack/reconciler/ReactCompositeComponent.js", funcName: "_renderValidatedComponentWithoutOwnerOrContext", line: intPtr(1050), column: intPtr(29)},
				{url: "/home/username/sample-workspace/sampleapp.collect.react/node_modules/react-native/Libraries/Renderer/src/renderers/shared/stack/reconciler/ReactCompositeComponent.js", funcName: "_renderValidatedComponent", line: intPtr(1075), column: intPtr(15)},
				{url: "/home/username/sample-workspace/sampleapp.collect.react/node_modules/react-native/Libraries/Renderer/src/renderers/shared/stack/reconciler/ReactCompositeComponent.js", funcName: "renderedElement", line: intPtr(484), column: intPtr(29)},
				{url: "/home/username/sample-workspace/sampleapp.collect.react/node_modules/react-native/Libraries/Renderer/src/renderers/shared/stack/reconciler/ReactCompositeComponent.js", funcName: "_currentElement", line: intPtr(346), column: intPtr(40)},
				{url: "/home/username/sample-workspace/sampleapp.collect.react/node_modules/react-native/Libraries/Renderer/src/renderers/shared/stack/reconciler/ReactReconciler.js", funcName: "child", line: intPtr(68), column: intPtr(25)},
				{url: "/home/username/sample-workspace/sampleapp.collect.react/node_modules/react-native/Libraries/Renderer/src/renderers/shared/stack/reconciler/ReactMultiChild.js", funcName: "children", line: intPtr(264), column: intPtr(10)},
				{url: "/home/username/sample-workspace/sampleapp.collect.react/node_modules/react-native/Libraries/Renderer/src/renderers/native/ReactNativeBaseComponent.js", funcName: "this", line: intPtr(74), column: intPtr(41)},
			},
			expectedMode: parseModeStack,
		},
		{
			name:          "Android React Native Production error",
			exceptionName: "Error",
			exceptionMsg:  "Error: test",
			stack: "Error: test\n" +
				"value@index.android.bundle:12:1917\n" +
				"onPress@index.android.bundle:12:2336\n" +
				"touchableHandlePress@index.android.bundle:258:1497\n" +
				"[native code]\n" +
				"_performSideEffectsForTransition@index.android.bundle:252:8508",
			expectedName:    "Error",
			expectedMessage: "Error: test",
			expectedFrames: []stackFrame{
				{url: "index.android.bundle", funcName: "value", line: intPtr(12), column: intPtr(1917)},
				{url: "index.android.bundle", funcName: "onPress", line: intPtr(12), column: intPtr(2336)},
				{url: "index.android.bundle", funcName: "touchableHandlePress", line: intPtr(258), column: intPtr(1497)},
				{url: "[native code]", funcName: unknownFunction, line: nil, column: nil},
				{url: "index.android.bundle", funcName: "_performSideEffectsForTransition", line: intPtr(252), column: intPtr(8508)},
			},
			expectedMode: parseModeStack,
		},

		// Edge Cases
		{
			name:          "Chrome/V8 format with anonymous functions",
			exceptionName: "Error",
			exceptionMsg:  "",
			stack: "  Error: \n" +
				"    at new <anonymous> (http://example.com/js/test.js:63:1)\n" +
				"    at namedFunc0 (http://example.com/js/script.js:10:2)\n" +
				"    at http://example.com/js/test.js:65:10\n" +
				"    at namedFunc2 (http://example.com/js/script.js:20:5)\n" +
				"    at http://example.com/js/test.js:67:5\n" +
				"    at namedFunc4 (http://example.com/js/script.js:100001:10002)",
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
		{
			name:          "Stack trace with native frames",
			exceptionName: "Error",
			exceptionMsg:  "Test error",
			stack: "Error: Test error\n" +
				"   at Array.map (native)\n" +
				"   at funcA (fileA.js:10:15)\n" +
				"   at Array.forEach (native)\n" +
				"   at funcB (fileB.js:20:25)",
			expectedName:    "Error",
			expectedMessage: "Test error",
			expectedFrames: []stackFrame{
				{url: "", funcName: "Array.map", line: nil, column: nil},
				{url: "fileA.js", funcName: "funcA", line: intPtr(10), column: intPtr(15)},
				{url: "", funcName: "Array.forEach", line: nil, column: nil},
				{url: "fileB.js", funcName: "funcB", line: intPtr(20), column: intPtr(25)},
			},
			expectedMode: parseModeStack,
		},
		{
			name:            "Empty stack trace",
			exceptionName:   "Error",
			exceptionMsg:    "Error message",
			stack:           "",
			expectedName:    "Error",
			expectedMessage: "Error message",
			expectedFrames:  []stackFrame{},
			expectedMode:    parseModeFailed,
		},
		{
			name:            "Unparseable stack trace",
			exceptionName:   "Error",
			exceptionMsg:    "Error message",
			stack:           "This is not a valid stack trace format\nSome random text\nMore random text",
			expectedName:    "Error",
			expectedMessage: "Error message",
			expectedFrames:  []stackFrame{},
			expectedMode:    parseModeFailed,
		},
		{
			name:          "Chrome with query string URL",
			exceptionName: "Error",
			exceptionMsg:  "Test error",
			stack: "Error: Test error\n" +
				"    at foo (http://example.com/file.js?v=123:10:5)\n" +
				"    at bar (http://example.com/file.js?v=123&debug=true:20:10)",
			expectedName:    "Error",
			expectedMessage: "Test error",
			expectedFrames: []stackFrame{
				{url: "http://example.com/file.js?v=123", funcName: "foo", line: intPtr(10), column: intPtr(5)},
				{url: "http://example.com/file.js?v=123&debug=true", funcName: "bar", line: intPtr(20), column: intPtr(10)},
			},
			expectedMode: parseModeStack,
		},
		{
			name:          "Chrome with fragment URL",
			exceptionName: "Error",
			exceptionMsg:  "Test error",
			stack: "Error: Test error\n" +
				"    at foo (http://example.com/file.js#section:10:5)\n" +
				"    at bar (http://example.com/file.js#top:20:10)",
			expectedName:    "Error",
			expectedMessage: "Test error",
			expectedFrames: []stackFrame{
				{url: "http://example.com/file.js#section", funcName: "foo", line: intPtr(10), column: intPtr(5)},
				{url: "http://example.com/file.js#top", funcName: "bar", line: intPtr(20), column: intPtr(10)},
			},
			expectedMode: parseModeStack,
		},
		{
			name:          "Chrome extension error",
			exceptionName: "Error",
			exceptionMsg:  "Extension error",
			stack: "Error: Extension error\n" +
				"    at foo (chrome-extension://abc123def456/script.js:10:5)\n" +
				"    at bar (chrome-extension://abc123def456/background.js:20:10)",
			expectedName:    "Error",
			expectedMessage: "Extension error",
			expectedFrames: []stackFrame{
				{url: "chrome-extension://abc123def456/script.js", funcName: "foo", line: intPtr(10), column: intPtr(5)},
				{url: "chrome-extension://abc123def456/background.js", funcName: "bar", line: intPtr(20), column: intPtr(10)},
			},
			expectedMode: parseModeStack,
		},
		{
			name:          "Incomplete URL due to missing closing paren",
			exceptionName: "Error",
			exceptionMsg:  "Test error",
			stack: "Error: Test error\n" +
				"    at func (http://example.com/file.js:10:5\n" +
				"    at func2 (http://example.com/file2.js:20:1)",
			expectedName:    "Error",
			expectedMessage: "Test error",
			expectedFrames: []stackFrame{
				{url: "http://example.com/file.js", funcName: "func", line: intPtr(10), column: intPtr(5)},
				{url: "http://example.com/file2.js", funcName: "func2", line: intPtr(20), column: intPtr(1)},
			},
			expectedMode: parseModeStack,
		},
		{
			name:          "non-numeric line number",
			exceptionName: "Error",
			exceptionMsg:  "Test error",
			stack: "Error: Test error\n" +
				"    at func (http://example.com/file.js:abc:5)\n" +
				"    at func2 (http://example.com/file2.js:20:1)",
			expectedName:    "Error",
			expectedMessage: "Test error",
			expectedFrames: []stackFrame{
				{url: "http://example.com/file.js:abc", funcName: "func", line: intPtr(5), column: nil},
				{url: "http://example.com/file2.js", funcName: "func2", line: intPtr(20), column: intPtr(1)},
			},
			expectedMode: parseModeStack,
		},
		{
			name:          "non-numeric column number",
			exceptionName: "Error",
			exceptionMsg:  "Test error",
			stack: "Error: Test error\n" +
				"    at func (http://example.com/file.js:10:xyz)\n" +
				"    at func2 (http://example.com/file2.js:20:1)",
			expectedName:    "Error",
			expectedMessage: "Test error",
			expectedFrames: []stackFrame{
				{url: "http://example.com/file.js:10:xyz", funcName: "func", line: nil, column: nil},
				{url: "http://example.com/file2.js", funcName: "func2", line: intPtr(20), column: intPtr(1)},
			},
			expectedMode: parseModeStack,
		},
		{
			name:          "Line and column numbers at zero",
			exceptionName: "Error",
			exceptionMsg:  "Test error",
			stack: "Error: Test error\n" +
				"    at func (http://example.com/file.js:0:1)\n" +
				"    at func2 (http://example.com/file.js:1:0)",
			expectedName:    "Error",
			expectedMessage: "Test error",
			expectedFrames: []stackFrame{
				{url: "http://example.com/file.js", funcName: "func", line: intPtr(0), column: intPtr(1)},
				{url: "http://example.com/file.js", funcName: "func2", line: intPtr(1), column: intPtr(0)},
			},
			expectedMode: parseModeStack,
		},
		{
			name:          "Line and column at max uint32",
			exceptionName: "Error",
			exceptionMsg:  "Test error",
			stack: fmt.Sprintf("Error: Test error\n"+
				"    at func (http://example.com/file.js:%d:%d)", math.MaxUint32, math.MaxUint32),
			expectedName:    "Error",
			expectedMessage: "Test error",
			expectedFrames: []stackFrame{
				{url: "http://example.com/file.js", funcName: "func", line: intPtr(int(math.MaxUint32)), column: intPtr(int(math.MaxUint32))},
			},
			expectedMode: parseModeStack,
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
