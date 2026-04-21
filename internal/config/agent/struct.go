package configuration

import (
	"github.com/spf13/viper"
)

type Config struct {
	Database struct {
		Path string `mapstructure:"path"`
	} `mapstructure:"database"`
	Api struct {
		Address string `mapstructure:"address"`
		Port    int    `mapstructure:"port"`
	} `mapstructure:"api"`
	Prometheus struct {
		Address string `mapstructure:"address"`
		Port    int    `mapstructure:"port"`
	} `mapstructure:"prometheus"`
	Worker struct {
		Count      int `mapstructure:"count"`
		BufferSize int `mapstructure:"buffer_size"`
	} `mapstructure:"worker"`
	DefaultInterface string            `mapstructure:"default_interface"`
	Interfaces       map[string]string `mapstructure:"interfaces"`
}

func LoadConfig(path string) (*Config, error) {
	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")

	v.SetDefault("database.path", "/var/lib/two/data/")
	v.SetDefault("api.address", "")
	v.SetDefault("api.port", 8080)
	v.SetDefault("prometheus.address", "")
	v.SetDefault("prometheus.port", 9090)
	v.SetDefault("worker.count", 4)
	v.SetDefault("worker.buffer_size", 100)
	v.SetDefault("default_interface", "br-000000")

	v.ReadInConfig()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
