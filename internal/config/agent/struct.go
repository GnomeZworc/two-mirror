package configuration

import (
	"github.com/spf13/viper"
)

type Config struct {
	Database struct {
		Path string `mapstructure:"path"`
	} `mapstructure:"database"`
}

func LoadConfig(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	v.SetDefault("database.path", "/var/lib/two/data/")

	v.ReadInConfig()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
