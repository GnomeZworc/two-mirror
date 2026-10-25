package configuration

import (
	"strings"
	"testing"
	"time"
)

func TestLoadConfig_TimesDefauts(t *testing.T) {
	cfg, err := LoadConfig(writeYAML(t, ""))
	if err != nil {
		t.Fatalf("LoadConfig a échoué : %v", err)
	}
	want := Times{
		TimeoutVMsStop:     300,
		NextVMsShutdown:    30,
		TimeoutSubnetStop:  360,
		TimeoutVPCStop:     420,
		TimeoutSubnetStart: 300,
		TimeoutVMsStart:    300,
		PollInterval:       2,
	}
	if cfg.Times != want {
		t.Errorf("times par défaut : %+v, attendu %+v", cfg.Times, want)
	}
}

func TestLoadConfig_TimesValeursExplicites(t *testing.T) {
	cfg, err := LoadConfig(writeYAML(t, "times:\n  timeout_vms_stop: 600\n  next_vms_shutdown: 10\n  timeout_subnet_stop: 700\n  timeout_vpc_stop: 800\n  timeout_subnet_start: 50\n  timeout_vms_start: 60\n  poll_interval: 1\n"))
	if err != nil {
		t.Fatalf("LoadConfig a échoué : %v", err)
	}
	want := Times{600, 10, 700, 800, 50, 60, 1}
	if cfg.Times != want {
		t.Errorf("times : %+v, attendu %+v", cfg.Times, want)
	}
}

func TestLoadConfig_AncienneSectionDispatcherIgnoree(t *testing.T) {
	cfg, err := LoadConfig(writeYAML(t, "dispatcher:\n  timeout_seconds: 900\n  poll_seconds: 7\n"))
	if err != nil {
		t.Fatalf("une section dispatcher restante ne doit pas empêcher le démarrage : %v", err)
	}
	if cfg.Times.TimeoutVMsStop != 300 || cfg.Times.PollInterval != 2 {
		t.Errorf("dispatcher.* ne doit rien changer : %+v", cfg.Times)
	}
}

func TestLoadConfig_TimesInvalidesRefuses(t *testing.T) {
	for _, c := range []struct{ yaml, msg string }{
		{"times:\n  timeout_vms_stop: 0\n", "times.timeout_vms_stop 0 must be a positive number of seconds"},
		{"times:\n  next_vms_shutdown: -1\n", "times.next_vms_shutdown -1 must be a positive number of seconds"},
		{"times:\n  timeout_subnet_stop: 0\n", "times.timeout_subnet_stop 0"},
		{"times:\n  timeout_vpc_stop: 0\n", "times.timeout_vpc_stop 0"},
		{"times:\n  timeout_subnet_start: 0\n", "times.timeout_subnet_start 0"},
		{"times:\n  timeout_vms_start: 0\n", "times.timeout_vms_start 0"},
		{"times:\n  poll_interval: 0\n", "times.poll_interval 0"},
		{"times:\n  next_vms_shutdown: 300\n", "times.next_vms_shutdown 300 must be below times.timeout_vms_stop 300"},
	} {
		if _, err := LoadConfig(writeYAML(t, c.yaml)); err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%q : erreur %v, attendu %q", c.yaml, err, c.msg)
		}
	}
	if _, err := LoadConfig(writeYAML(t, "times:\n  next_vms_shutdown: 299\n")); err != nil {
		t.Errorf("next_vms_shutdown 299 sous timeout_vms_stop 300 : %v", err)
	}
}

func TestTimesDurees(t *testing.T) {
	var zero Times
	cases := []struct {
		name      string
		got, want time.Duration
	}{
		{"VMsStop", zero.VMsStop(), 300 * time.Second},
		{"VMsShutdown", zero.VMsShutdown(), 30 * time.Second},
		{"SubnetStop", zero.SubnetStop(), 360 * time.Second},
		{"VPCStop", zero.VPCStop(), 420 * time.Second},
		{"SubnetStart", zero.SubnetStart(), 300 * time.Second},
		{"VMsStart", zero.VMsStart(), 300 * time.Second},
		{"Poll", zero.Poll(), 2 * time.Second},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("Times{}.%s() = %v, attendu %v", c.name, c.got, c.want)
		}
	}
	set := Times{11, 12, 13, 14, 15, 16, 17}
	got := []time.Duration{set.VMsStop(), set.VMsShutdown(), set.SubnetStop(), set.VPCStop(), set.SubnetStart(), set.VMsStart(), set.Poll()}
	for i, d := range got {
		if want := time.Duration(11+i) * time.Second; d != want {
			t.Errorf("durée %d = %v, attendu %v", i, d, want)
		}
	}
}
