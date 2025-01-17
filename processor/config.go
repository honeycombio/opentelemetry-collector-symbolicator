package symbolicatorprocessor

import "time"

// Config defines configuration for the symbolicator processor.
type Config struct {
	// ColumnsAttributeKey is the attribute key that contains the column numbers
	// of the stack trace.
	ColumnsAttributeKey string `mapstructure:"columns_attribute_key"`

	// FunctionsAttributeKey is the attribute key that contains the function
	// names of the stack trace.
	FunctionsAttributeKey string `mapstructure:"functions_attribute_key"`

	// LinesAttributeKey is the attribute key that contains the line numbers of
	// the stack trace.
	LinesAttributeKey string `mapstructure:"lines_attribute_key"`

	// UrlsAttributeKey is the attribute key that contains the URLs of the stack
	// trace.
	UrlsAttributeKey string `mapstructure:"urls_attribute_key"`

	// OutputStackTraceKey is the attribute key that the symbolicated stack trace
	// will be written to.
	OutputStackTraceKey string `mapstructure:"output_stack_trace_key"`

	// StackTypeKey is the attribute key that contains the type of the stack trace.
	StackTypeKey string `mapstructure:"stack_type_key"`

	// StackMessageKey is the attribute key that contains the message of the stack trace.
	StackMessageKey string `mapstructure:"stack_message_key"`

	// preserveStackTrace is a config option that determines whether to keep the
	// original stack trace in the output.
	PreserveStackTrace bool `mapstructure:"preserve_stack_trace"`

	// OriginalStackTraceKey is the attribute key that preserves the original stack
	// trace.
	OriginalStackTraceKey string `mapstructure:"original_stack_trace_key"`

	// OriginalColumnsAttributeKey is the attribute key that preserves the original
	// column numbers.
	OriginalColumnsAttributeKey string `mapstructure:"original_columns_attribute_key"`

	// OriginalFunctionsAttributeKey is the attribute key that preserves the original
	// function names.
	OriginalFunctionsAttributeKey string `mapstructure:"original_functions_attribute_key"`

	// OriginalLinesAttributeKey is the attribute key that preserves the original
	// line numbers.
	OriginalLinesAttributeKey string `mapstructure:"original_lines_attribute_key"`

	// OriginalUrlsAttributeKey is the attribute key that preserves the original URLs.
	OriginalUrlsAttributeKey string `mapstructure:"original_urls_attribute_key"`

	// SourceMapFilePath is a file path to where the minified source and source
	// maps are stored on disk.
	SourceMapFilePath string `mapstructure:"source_map_file_path"`

	// Timeout is the maximum time to wait for a response from the symbolicator.
	Timeout time.Duration `mapstructure:"timeout"`
}

// Validate checks the configuration for any issues.
func (c *Config) Validate() error {
	return nil
}
