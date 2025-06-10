package dsymprocessor

import "time"

// Config defines configuration for the symbolicator processor.
type Config struct {
	// SymbolicatorFailureAttributeKey is the attribute key that will be set to
	// true if the symbolicator fails to fully symbolicate a stack trace.
	SymbolicatorFailureAttributeKey string `mapstructure:"symbolicator_failure_attribute_key"`

	// MetricKitStackTraceAttributeKey is the attribute key that contains the metrickit
	// stack trace.
	MetricKitStackTraceAttributeKey string `mapstructure:"metrickit_stack_trace_attribute_key"`

	// OutputMetricKitStackTraceAttributeKey is the attribute key that contains the
	// symbolicated metrickit stack trace.
	OutputMetricKitStackTraceAttributeKey string `mapstructure:"output_metrickit_stack_trace_attribute_key"`

	// preserveStackTrace is a config option that determines whether to keep the
	// original stack trace in the output.
	// TODO: use this
	PreserveStackTrace bool `mapstructure:"preserve_stack_trace"`

	DSYMStoreKey string `mapstructure:"dsym_store"`

	// LocalSourceMapConfiguration is the configuration for sourcing source maps on a local volume.
	LocalSourceMapConfiguration *LocalSourceMapConfiguration `mapstructure:"local_dsyms"`

	// S3SourceMapConfiguration is the configuration for sourcing source maps from S3.
	S3SourceMapConfiguration *S3SourceMapConfiguration `mapstructure:"s3_dsyms"`

	// GCSSourceMapConfiguration is the configuration for sourcing source maps from GCS.
	GCSSourceMapConfiguration *GCSSourceMapConfiguration `mapstructure:"gcs_dsyms"`

	// Timeout is the maximum time to wait for a response from the symbolicator.
	Timeout time.Duration `mapstructure:"timeout"`

	// CacheSize is the maximum number of dSYMs to cache.
	DSYMCacheSize int `mapstructure:"dsym_cache_size"`
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
