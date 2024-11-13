package symbolicatorprocessor

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
}

// Validate checks the configuration for any issues.
func (c *Config) Validate() error {
	return nil
}
