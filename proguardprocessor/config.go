package proguardprocessor

import "time"

type Config struct {
	// SymbolicatorFailureAttributeKey is the attribute key that will be set to
	// true if the symbolicator fails to fully symbolicate a stack trace.
	SymbolicatorFailureAttributeKey string `mapstructure:"symbolicator_failure_attribute_key"`

	// SymbolicatorErrorAttributeKey is the attribute key that contains the error
	// message if the symbolicator fails to fully symbolicate a stack trace.
	SymbolicatorErrorAttributeKey string `mapstructure:"symbolicator_error_attribute_key"`

	// SymbolicatorParsingMethodAttributeKey is the attribute key that contains the
	// method used to parse the stack trace.
	SymbolicatorParsingMethodAttributeKey string `mapstructure:"symbolicator_parsing_method_attribute_key"`

	// ClassesAttributeKey is the attribute key that contains the class names
	// of the stack trace.
	ClassesAttributeKey string `mapstructure:"classes_attribute_key"`

	// MethodsAttributeKey is the attribute key that contains the method
	// names of the stack trace.
	MethodsAttributeKey string `mapstructure:"methods_attribute_key"`

	// LinesAttributeKey is the attribute key that contains the line numbers of
	// the stack trace.
	LinesAttributeKey string `mapstructure:"lines_attribute_key"`

	// SourceFilesAttributeKey is the attribute key that contains the source file names
	// of the stack trace.
	SourceFilesAttributeKey string `mapstructure:"source_files_attribute_key"`

	// StackTraceAttributeKey is the attribute key for the stack trace.
	// When structured stack trace attributes are missing, the processor reads the raw
	// stack trace from this key and parses it.
	// The symbolicated stack trace is written back to this key.
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

	// OriginalClassesAttributeKey is the attribute key that preserves the original class
	// names.
	OriginalClassesAttributeKey string `mapstructure:"original_classes_attribute_key"`

	// OriginalMethodsAttributeKey is the attribute key that preserves the original method
	// names.
	OriginalMethodsAttributeKey string `mapstructure:"original_methods_attribute_key"`

	// OriginalLinesAttributeKey is the attribute key that preserves the original
	// line numbers.
	OriginalLinesAttributeKey string `mapstructure:"original_lines_attribute_key"`

	// OriginalSourceFilesAttributeKey is the attribute key that preserves the original
	// source file names.
	OriginalSourceFilesAttributeKey string `mapstructure:"original_source_files_attribute_key"`

	// ProguardUUIDAttributeKey is the attribute key that contains the UUID
	// of the proguard mapping file.
	// This is used to identify which proguard mapping file to use for symbolication.
	ProguardUUIDAttributeKey string `mapstructure:"proguard_uuid_attribute_key"`

	ProguardStoreKey string `mapstructure:"proguard_store"`

	// LocalProguardConfiguration is the configuration for sourcing proguard files on a local volume.
	LocalProguardConfiguration *LocalStoreConfiguration `mapstructure:"local_store"`

	// S3ProguardConfiguration is the configuration for sourcing proguard files from S3.
	S3ProguardConfiguration *S3StoreConfiguration `mapstructure:"s3_store"`

	// GCSProguardConfiguration is the configuration for sourcing proguard files from GCS.
	GCSProguardConfiguration *GCSStoreConfiguration `mapstructure:"gcs_store"`

	// Timeout is the maximum time to wait for a response from the symbolicator.
	Timeout time.Duration `mapstructure:"timeout"`

	// CacheSize is the maximum number of proguard files to cache.
	ProguardCacheSize int `mapstructure:"proguard_cache_size"`

	// LanguageAttributeKey is the attribute key that contains the programming language
	// or SDK language of the telemetry signal (e.g., "telemetry.sdk.language").
	// This is used to determine if this processor should handle the signal.
	LanguageAttributeKey string `mapstructure:"language_attribute_key"`

	// AllowedLanguages is a list of language values that this processor will handle.
	// If the signal's language attribute matches any value in this list, the processor will run.
	// If empty (default), the processor will process all signals regardless of language.
	AllowedLanguages []string `mapstructure:"allowed_languages"`
}

type LocalStoreConfiguration struct {
	// Path is a file path to where the proguard files are stored on disk.
	Path string `mapstructure:"path"`
}

type S3StoreConfiguration struct {
	// Region is the AWS region where the S3 bucket is located.
	Region string `mapstructure:"region"`
	// BucketName is the name of the S3 bucket.
	BucketName string `mapstructure:"bucket"`
	// Prefix is the prefix to use when looking for proguard files.
	Prefix string `mapstructure:"prefix"`
}

type GCSStoreConfiguration struct {
	// BucketName is the name of the GCS bucket.
	BucketName string `mapstructure:"bucket"`
	// Prefix is the prefix to use when looking for proguard files.
	Prefix string `mapstructure:"prefix"`
}

func (c *Config) Validate() error {
	return nil
}
