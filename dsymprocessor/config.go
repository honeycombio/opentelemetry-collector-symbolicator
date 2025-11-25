package dsymprocessor

import "time"

// Config defines configuration for the symbolicator processor.
type Config struct {
	// SymbolicatorFailureAttributeKey is the attribute key that will be set to
	// true if the symbolicator fails to fully symbolicate a stack trace.
	SymbolicatorFailureAttributeKey string `mapstructure:"symbolicator_failure_attribute_key"`

	// SymbolicatorErrorAttributeKey is the attribute key that contains the error
	// message if the symbolicator fails to fully symbolicate a stack trace.
	SymbolicatorErrorAttributeKey string `mapstructure:"symbolicator_error_attribute_key"`

	// StackTraceAttributeKey is the attribute key that contains an explicitly thrown stack trace.
	StackTraceAttributeKey string `mapstructure:"stack_trace_attribute_key"`

	// OriginalStackTraceKey is the attribute key that preserves the original stack
	// trace.
	OriginalStackTraceKey string `mapstructure:"original_stack_trace_key"`

	// BuildUUIDAttributeKey is the attribute key that contains the build UUID of the current app.
	BuildUUIDAttributeKey string `mapstructure:"build_uuid_attribute_key"`

	// AppExecutableAttributeKey is the attribute key that contains the current app's executable name.
	AppExecutableAttributeKey string `mapstructure:"app_executable_attribute_key"`

	// MetricKitStackTraceAttributeKey is the attribute key that contains the metrickit
	// stack trace.
	MetricKitStackTraceAttributeKey string `mapstructure:"metrickit_stack_trace_attribute_key"`

	// OutputMetricKitStackTraceAttributeKey is the attribute key that contains the
	// symbolicated metrickit stack trace.
	OutputMetricKitStackTraceAttributeKey string `mapstructure:"output_metrickit_stack_trace_attribute_key"`

	// OutputMetricKitStackTraceAttributeKey is the attribute key that contains the
	// inferred  metrickit stack trace.
	OutputMetricKitExceptionTypeAttributeKey string `mapstructure:"output_metrickit_exception_type_attribute_key"`

	// OutputMetricKitStackTraceAttributeKey is the attribute key that contains the
	// symbolicated metrickit stack trace.
	OutputMetricKitExceptionMessageAttributeKey string `mapstructure:"output_metrickit_exception_message_attribute_key"`

	// preserveStackTrace is a config option that determines whether to keep the
	// original stack trace in the output.
	PreserveStackTrace bool `mapstructure:"preserve_stack_trace"`

	DSYMStoreKey string `mapstructure:"dsym_store"`

	// LocalDSYMConfiguration is the configuration for sourcing source maps on a local volume.
	LocalDSYMConfiguration *LocalDSYMConfiguration `mapstructure:"local_dsyms"`

	// S3DSYMConfiguration is the configuration for sourcing source maps from S3.
	S3DSYMConfiguration *S3DSYMConfiguration `mapstructure:"s3_dsyms"`

	// GCSDSYMConfiguration is the configuration for sourcing source maps from GCS.
	GCSDSYMConfiguration *GCSDSYMConfiguration `mapstructure:"gcs_dsyms"`

	// Timeout is the maximum time to wait for a response from the symbolicator.
	Timeout time.Duration `mapstructure:"timeout"`

	// CacheSize is the maximum number of dSYMs to cache.
	DSYMCacheSize int `mapstructure:"dsym_cache_size"`
}

type LocalDSYMConfiguration struct {
	// Path is a file path to where the dSYMs are stored on disk.
	Path string `mapstructure:"path"`
}

type S3DSYMConfiguration struct {
	// Region is the AWS region where the S3 bucket is located.
	Region string `mapstructure:"region"`
	// BucketName is the name of the S3 bucket.
	BucketName string `mapstructure:"bucket"`
	// Prefix is the prefix to use when looking for dSYMs.
	Prefix string `mapstructure:"prefix"`
}

type GCSDSYMConfiguration struct {
	// BucketName is the name of the GCS bucket.
	BucketName string `mapstructure:"bucket"`
	// Prefix is the prefix to use when looking for dSYMs.
	Prefix string `mapstructure:"prefix"`
}

// Validate checks the configuration for any issues.
func (c *Config) Validate() error {
	return nil
}
