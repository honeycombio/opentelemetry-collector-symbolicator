package uploaderextension

type Config struct {
	Endpoint         string `mapstructure:"endpoint"`
	StorageDirectory string `mapstructure:"storage_directory"`
}
