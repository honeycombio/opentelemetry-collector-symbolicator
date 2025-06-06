package proguardprocessor

type Config struct {
	// ClassesAttributeKey is the attribute key that contains the class names
	// of the stack trace.
	ClassesAttributeKey string `mapstructure:"classes_attribute_key"`

	// MethodsAttributeKey is the attribute key that contains the method
	// names of the stack trace.
	MethodsAttributeKey string `mapstructure:"methods_attribute_key"`

	// LinesAttributeKey is the attribute key that contains the line numbers of
	// the stack trace.
	LinesAttributeKey string `mapstructure:"lines_attribute_key"`
}

func (c *Config) Validate() error {
	return nil
}
