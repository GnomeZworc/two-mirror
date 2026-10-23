package configuration

import (
	"strings"
	"testing"
)

func TestLoadConfig_NetworkDefauts(t *testing.T) {
	cfg, err := LoadConfig(writeYAML(t, ""))
	if err != nil {
		t.Fatalf("LoadConfig a échoué : %v", err)
	}
	if cfg.Network.MTUVM != 1500 || cfg.Network.AllowMTUVMBelow1500 || cfg.Network.AllowMTUUnderlayBelow9000 {
		t.Errorf("network par défaut : %+v", cfg.Network)
	}
}

func TestLoadConfig_MTUVMRefuseSansOption(t *testing.T) {
	if _, err := LoadConfig(writeYAML(t, "network:\n  mtu_vm: 1450\n")); err == nil || !strings.Contains(err.Error(), "allow_mtu_vm_below_1500") {
		t.Errorf("mtu_vm 1450 sans option : %v", err)
	}
	cfg, err := LoadConfig(writeYAML(t, "network:\n  mtu_vm: 1450\n  allow_mtu_vm_below_1500: true\n"))
	if err != nil || cfg.Network.MTUVM != 1450 {
		t.Errorf("mtu_vm 1450 avec option : %v, %+v", err, cfg)
	}
}

func TestNetworkValidate(t *testing.T) {
	cases := []struct {
		n  Network
		ok bool
	}{
		{Network{MTUVM: 1500}, true},
		{Network{MTUVM: 1501}, false},
		{Network{MTUVM: 9000, AllowMTUVMBelow1500: true}, false},
		{Network{MTUVM: 1499}, false},
		{Network{MTUVM: 1499, AllowMTUVMBelow1500: true}, true},
		{Network{MTUVM: 1280, AllowMTUVMBelow1500: true}, true},
		{Network{MTUVM: 1279, AllowMTUVMBelow1500: true}, false},
		{Network{MTUVM: 0, AllowMTUVMBelow1500: true}, false},
	}
	for _, c := range cases {
		if err := c.n.Validate(); (err == nil) != c.ok {
			t.Errorf("Validate(%+v) = %v", c.n, err)
		}
	}
}

func TestNetworkCheckUnderlay(t *testing.T) {
	n := Network{MTUVM: 1500}
	if err := n.CheckUnderlay("br-000000", 9000); err != nil {
		t.Errorf("9000 : %v", err)
	}
	for _, m := range []int{9216, 8999, 1550} {
		if err := n.CheckUnderlay("br-000000", m); err == nil || !strings.Contains(err.Error(), "allow_mtu_underlay_below_9000") {
			t.Errorf("%d sans option : %v", m, err)
		}
	}
	n.AllowMTUUnderlayBelow9000 = true
	if err := n.CheckUnderlay("br-000000", 1550); err != nil {
		t.Errorf("1550 avec option : %v", err)
	}
	if err := n.CheckUnderlay("br-000000", 1549); err == nil || !strings.Contains(err.Error(), "vxlan header") {
		t.Errorf("1549 avec option, mtu_vm 1500 : %v", err)
	}
	n.MTUVM = 1400
	if err := n.CheckUnderlay("br-000000", 1450); err != nil {
		t.Errorf("1450 pour mtu_vm 1400 : %v", err)
	}
	if err := (Network{MTUVM: 8960}).CheckUnderlay("br-000000", 9000); err == nil {
		t.Error("mtu_vm 8960 + 50 dépasse 9000")
	}
}

func TestNetworkVMIs1500WhenUnset(t *testing.T) {
	if got := (Network{}).VM(); got != 1500 {
		t.Errorf("VM() d'une section vide = %d", got)
	}
	if got := (Network{MTUVM: 1400}).VM(); got != 1400 {
		t.Errorf("VM() = %d, attendu 1400", got)
	}
	if err := (Network{}).CheckUnderlay("br-000000", 1549); err == nil {
		t.Error("section vide : 1549 doit être refusé (mtu_vm 1500 + 50)")
	}
}

func TestNetworkCheckBridge(t *testing.T) {
	n := Network{MTUVM: 1500}
	if err := n.CheckBridge("br-lan", 1500); err != nil {
		t.Errorf("1500 : %v", err)
	}
	if err := n.CheckBridge("br-000000", 9000); err != nil {
		t.Errorf("9000 : %v", err)
	}
	if err := n.CheckBridge("br-lan", 1499); err == nil || !strings.Contains(err.Error(), "br-lan") {
		t.Errorf("1499 : %v", err)
	}
}
