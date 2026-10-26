package configuration

import (
	"errors"
	"fmt"
	"os"

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
	Times  Times `mapstructure:"times"`
	Logger struct {
		Level string `mapstructure:"level"`
		Debug bool   `mapstructure:"debug"`
	} `mapstructure:"logger"`
	Metadata struct {
		RunDir string `mapstructure:"run_dir"`
	} `mapstructure:"metadata"`
	DHCP struct {
		Backend string `mapstructure:"backend"`
	} `mapstructure:"dhcp"`
	Admin struct {
		Enabled bool   `mapstructure:"enabled"`
		Address string `mapstructure:"address"`
		Port    int    `mapstructure:"port"`
	} `mapstructure:"admin"`
	QEMU struct {
		OVMFCodePath     string `mapstructure:"ovmf_code_path"`
		OVMFVarsTemplate string `mapstructure:"ovmf_vars_template"`
		UEFIVarsDir      string `mapstructure:"uefi_vars_dir"`
		SerialDir        string `mapstructure:"serial_dir"`
		MonitorDir       string `mapstructure:"monitor_dir"`
		QMPDir           string `mapstructure:"qmp_dir"`
	} `mapstructure:"qemu"`
	Watchdog struct {
		Enabled         bool `mapstructure:"enabled"`
		IntervalSeconds int  `mapstructure:"interval_seconds"`
	} `mapstructure:"watchdog"`
	Network          Network           `mapstructure:"network"`
	BGP              BGP               `mapstructure:"bgp"`
	HostSvc          HostSvc           `mapstructure:"hostsvc"`
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
	v.SetDefault("times.timeout_vms_stop", TimeoutVMsStop)
	v.SetDefault("times.next_vms_shutdown", NextVMsShutdown)
	v.SetDefault("times.timeout_subnet_stop", TimeoutSubnetStop)
	v.SetDefault("times.timeout_vpc_stop", TimeoutVPCStop)
	v.SetDefault("times.timeout_subnet_start", TimeoutSubnetStart)
	v.SetDefault("times.timeout_vms_start", TimeoutVMsStart)
	v.SetDefault("times.poll_interval", PollInterval)
	v.SetDefault("metadata.run_dir", "/run/two/metadata")
	v.SetDefault("dhcp.backend", BackendTwo)
	v.SetDefault("hostsvc.dns_upstream", []string{})
	v.SetDefault("hostsvc.ntp_upstream", []string{})
	v.SetDefault("hostsvc.rate_limit_per_vpc", RateLimitPerVPC)
	v.SetDefault("qemu.ovmf_code_path", "/usr/share/OVMF/OVMF_CODE.fd")
	v.SetDefault("qemu.ovmf_vars_template", "/usr/share/OVMF/OVMF_VARS.fd")
	v.SetDefault("qemu.uefi_vars_dir", "/run/two/vms/uefi")
	v.SetDefault("qemu.serial_dir", "/run/two/vms/serial")
	v.SetDefault("qemu.monitor_dir", "/run/two/vms/monitor")
	v.SetDefault("qemu.qmp_dir", "/run/two/vms/qmp")
	v.SetDefault("watchdog.enabled", false)
	v.SetDefault("watchdog.interval_seconds", 60)
	v.SetDefault("admin.enabled", false)
	v.SetDefault("admin.address", "127.0.0.1")
	v.SetDefault("admin.port", 9091)
	v.SetDefault("default_interface", "br-000000")
	v.SetDefault("logger.level", "info")
	v.SetDefault("logger.debug", false)
	v.SetDefault("network.mtu_vm", MTUVM)
	v.SetDefault("network.allow_mtu_vm_below_1500", false)
	v.SetDefault("network.allow_mtu_underlay_below_9000", false)

	if err := v.ReadInConfig(); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}
	if err := cfg.Network.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := cfg.Times.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := cfg.HostSvc.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	return &cfg, nil
}
