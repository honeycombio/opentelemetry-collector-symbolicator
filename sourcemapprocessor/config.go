package sourcemapprocessor

import (
	"time"
)

// Config defines configuration for the symbolicator processor.
type Config struct {
	// SymbolicatorFailureAttributeKey is the attribute key that will be set to
	// true if the symbolicator fails to fully symbolicate a stack trace.
	SymbolicatorFailureAttributeKey string `mapstructure:"symbolicator_failure_attribute_key"`

	// SymbolicatorErrorAttributeKey is the attribute key that contains the error
	// message if the symbolicator fails to fully symbolicate a stack trace.
	SymbolicatorErrorAttributeKey string `mapstructure:"symbolicator_error_attribute_key"`

	// SymbolicatorParsingMethodAttributeKey is the attribute key that contains the
	// method used to parse the stack trace (processor_parsed or structured_stacktrace_attributes).
	SymbolicatorParsingMethodAttributeKey string `mapstructure:"symbolicator_parsing_method_attribute_key"`

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

	// StackTraceAttributeKey is the attribute key that the symbolicated stack trace
	// will be written to.
	StackTraceAttributeKey string `mapstructure:"stack_trace_attribute_key"`

	// ExceptionTypeAttributeKey is the attribute key that contains the type of the exception.
	ExceptionTypeAttributeKey string `mapstructure:"exception_type_attribute_key"`

	// ExceptionMessageAttributeKey is the attribute key that contains the message of the exception.
	ExceptionMessageAttributeKey string `mapstructure:"exception_message_attribute_key"`

	// preserveStackTrace is a config option that determines whether to keep the
	// original stack trace in the output.
	PreserveStackTrace bool `mapstructure:"preserve_stack_trace"`

	// OriginalStackTraceAttributeKey is the attribute key that preserves the original stack
	// trace.
	OriginalStackTraceAttributeKey string `mapstructure:"original_stack_trace_attribute_key"`

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

	// BuildUUIDAttributeKey is the attribute key that contains the build UUID of the current app.
	BuildUUIDAttributeKey string `mapstructure:"build_uuid_attribute_key"`

	SourceMapStoreKey string `mapstructure:"source_map_store"`

	// LocalSourceMapConfiguration is the configuration for sourcing source maps on a local volume.
	LocalSourceMapConfiguration *LocalSourceMapConfiguration `mapstructure:"local_source_maps"`

	// S3SourceMapConfiguration is the configuration for sourcing source maps from S3.
	S3SourceMapConfiguration *S3SourceMapConfiguration `mapstructure:"s3_source_maps"`

	// GCSSourceMapConfiguration is the configuration for sourcing source maps from GCS.
	GCSSourceMapConfiguration *GCSSourceMapConfiguration `mapstructure:"gcs_source_maps"`

	// Timeout is the maximum time to wait for a response from the symbolicator.
	Timeout time.Duration `mapstructure:"timeout"`

	// CacheSize is the maximum number of source maps to cache.
	SourceMapCacheSize int `mapstructure:"source_map_cache_size"`

	// LanguageAttributeKey is the attribute key that contains the programming language
	// or SDK language of the telemetry signal (e.g., "telemetry.sdk.language").
	// This is used to determine if this processor should handle the signal.
	LanguageAttributeKey string `mapstructure:"language_attribute_key"`

	// AllowedLanguages is a list of language values that this processor will handle.
	// If the signal's language attribute matches any value in this list, the processor will run.
	// If empty (default), the processor will process all signals regardless of language.
	AllowedLanguages []string `mapstructure:"allowed_languages"`

	// EnableParityChecking enables parity checking mode where stacktraces are processed
	// through both the structured route (TraceKit) and the collector-side parsing route
	// (Sourcemap Processor) and the results are compared. Parity attributes are added
	// to the current span/log.
	// NOTE: This is for internal testing purposes only and will be removed in the future.
	EnableParityChecking bool `mapstructure:"enable_parity_checking"`
}

type LocalSourceMapConfiguration struct {
	// Path is a file path to where the minified source and source
	// maps are stored on disk.
	Path string `mapstructure:"path"`
}

type S3SourceMapConfiguration struct {
	// Region is the AWS region where the S3 bucket is located.
	Region string `mapstructure:"region"`
	// BucketName is the name of the S3 bucket.
	BucketName string `mapstructure:"bucket"`
	// Prefix is the prefix to use when looking for source maps.
	Prefix string `mapstructure:"prefix"`
}

type GCSSourceMapConfiguration struct {
	// BucketName is the name of the GCS bucket.
	BucketName string `mapstructure:"bucket"`
	// Prefix is the prefix to use when looking for source maps.
	Prefix string `mapstructure:"prefix"`
}

// Validate checks the configuration for any issues.
func (c *Config) Validate() error {
	return nil
}
