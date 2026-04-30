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
	Dispatcher struct {
		TimeoutSeconds int `mapstructure:"timeout_seconds"`
		PollSeconds    int `mapstructure:"poll_seconds"`
	} `mapstructure:"dispatcher"`
	Logger struct {
		Level string `mapstructure:"level"`
		Debug bool   `mapstructure:"debug"`
	} `mapstructure:"logger"`
	Metadata struct {
		RunDir string `mapstructure:"run_dir"`
	} `mapstructure:"metadata"`
	Admin struct {
		Enabled bool   `mapstructure:"enabled"`
		Address string `mapstructure:"address"`
		Port    int    `mapstructure:"port"`
	} `mapstructure:"admin"`
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
	v.SetDefault("dispatcher.timeout_seconds", 300)
	v.SetDefault("dispatcher.poll_seconds", 2)
	v.SetDefault("metadata.run_dir", "/run/two/metadata")
	v.SetDefault("admin.enabled", false)
	v.SetDefault("admin.address", "127.0.0.1")
	v.SetDefault("admin.port", 9091)
	v.SetDefault("default_interface", "br-000000")
	v.SetDefault("logger.level", "info")
	v.SetDefault("logger.debug", false)

	v.ReadInConfig()

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
