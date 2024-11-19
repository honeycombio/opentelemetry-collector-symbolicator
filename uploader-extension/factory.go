package uploaderextension

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
)

// Factory returns the factory for the extension.
func NewFactory() extension.Factory {
	return extension.NewFactory(
		component.MustNewType("uploaderextension"), // The extension type name
		createDefaultConfig,                        // Default config function
		createExtensionInstance,                    // Create instance function
		0,
	)
}

// CreateDefaultConfig creates the default configuration for extension. Notice
// that the default configuration is expected to fail for this extension.
func createDefaultConfig() component.Config {
	return &Config{
		Endpoint: "localhost:4000",
	}
}

func createExtensionInstance(ctx context.Context, settings extension.Settings, cfg component.Config) (extension.Extension, error) {
	config := cfg.(*Config)
	return newUploaderExtension(config), nil
}
