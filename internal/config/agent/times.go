package configuration

import (
	"fmt"
	"time"
)

const (
	TimeoutVMsStop     = 300
	NextVMsShutdown    = 30
	TimeoutSubnetStop  = 360
	TimeoutVPCStop     = 420
	TimeoutSubnetStart = 300
	TimeoutVMsStart    = 300
	PollInterval       = 2
)

type Times struct {
	TimeoutVMsStop     int `mapstructure:"timeout_vms_stop"`
	NextVMsShutdown    int `mapstructure:"next_vms_shutdown"`
	TimeoutSubnetStop  int `mapstructure:"timeout_subnet_stop"`
	TimeoutVPCStop     int `mapstructure:"timeout_vpc_stop"`
	TimeoutSubnetStart int `mapstructure:"timeout_subnet_start"`
	TimeoutVMsStart    int `mapstructure:"timeout_vms_start"`
	PollInterval       int `mapstructure:"poll_interval"`
}

func seconds(value, fallback int) time.Duration {
	if value == 0 {
		value = fallback
	}
	return time.Duration(value) * time.Second
}

func (t Times) VMsStop() time.Duration     { return seconds(t.TimeoutVMsStop, TimeoutVMsStop) }
func (t Times) VMsShutdown() time.Duration { return seconds(t.NextVMsShutdown, NextVMsShutdown) }
func (t Times) SubnetStop() time.Duration  { return seconds(t.TimeoutSubnetStop, TimeoutSubnetStop) }
func (t Times) VPCStop() time.Duration     { return seconds(t.TimeoutVPCStop, TimeoutVPCStop) }
func (t Times) SubnetStart() time.Duration { return seconds(t.TimeoutSubnetStart, TimeoutSubnetStart) }
func (t Times) VMsStart() time.Duration    { return seconds(t.TimeoutVMsStart, TimeoutVMsStart) }
func (t Times) Poll() time.Duration        { return seconds(t.PollInterval, PollInterval) }

func (t Times) Validate() error {
	for _, f := range []struct {
		key   string
		value int
	}{
		{"timeout_vms_stop", t.TimeoutVMsStop},
		{"next_vms_shutdown", t.NextVMsShutdown},
		{"timeout_subnet_stop", t.TimeoutSubnetStop},
		{"timeout_vpc_stop", t.TimeoutVPCStop},
		{"timeout_subnet_start", t.TimeoutSubnetStart},
		{"timeout_vms_start", t.TimeoutVMsStart},
		{"poll_interval", t.PollInterval},
	} {
		if f.value <= 0 {
			return fmt.Errorf("times.%s %d must be a positive number of seconds", f.key, f.value)
		}
	}
	if t.NextVMsShutdown >= t.TimeoutVMsStop {
		return fmt.Errorf("times.next_vms_shutdown %d must be below times.timeout_vms_stop %d", t.NextVMsShutdown, t.TimeoutVMsStop)
	}
	return nil
}
