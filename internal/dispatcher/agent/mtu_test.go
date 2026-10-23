package dispatcher

import (
	"strings"
	"testing"

	"git.g3e.fr/syonad/two/pkg/db/kv"
)

func withLinkMTU(t *testing.T, mtus map[string]int) {
	t.Helper()
	previous := LinkMTU
	LinkMTU = func(name string) (int, error) { return mtus[name], nil }
	t.Cleanup(func() { LinkMTU = previous })
}

func TestCreateSubnetCommand_Prepare_VxlanRefusesAnUnderlayOtherThan9000(t *testing.T) {
	withLinkMTU(t, map[string]int{"br-vms": 1500, "br-default": 9000})
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "running")

	cmd := CreateSubnetCommand{Name: "sn-1", VPC: "vpc-1", VxlanID: 100, IfaceType: "vms", CIDR: "10.0.0.0/24"}
	err := cmd.Prepare(db, testCfg())
	if err == nil || !strings.Contains(err.Error(), "underlay br-vms has mtu 1500") {
		t.Fatalf("underlay br-vms à 1500 : %v", err)
	}
	if _, err := kv.GetFromDB(db, "subnet/sn-1/state"); err == nil {
		t.Error("aucune clé ne doit être écrite après un refus")
	}

	cmd.IfaceType = ""
	if err := cmd.Prepare(db, testCfg()); err != nil {
		t.Errorf("default_interface à 9000 : %v", err)
	}
}

func TestCreateSubnetCommand_Prepare_VxlanUnderlayBelow9000WithTheOption(t *testing.T) {
	withLinkMTU(t, map[string]int{"br-vms": 1549})
	cfg := testCfg()
	cfg.Network.AllowMTUUnderlayBelow9000 = true
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "running")

	cmd := CreateSubnetCommand{Name: "sn-1", VPC: "vpc-1", VxlanID: 100, IfaceType: "vms", CIDR: "10.0.0.0/24"}
	if err := cmd.Prepare(db, cfg); err == nil || !strings.Contains(err.Error(), "vxlan header") {
		t.Fatalf("1549 pour mtu_vm 1500 : %v", err)
	}
	cfg.Network.MTUVM = 1499
	if err := cmd.Prepare(db, cfg); err != nil {
		t.Errorf("1549 pour mtu_vm 1499 : %v", err)
	}
}

func TestCreateSubnetCommand_Prepare_BridgeChecksTheBridgeWithoutVxlanHeader(t *testing.T) {
	withLinkMTU(t, map[string]int{"br-vms": 1500, "br-default": 1499})
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "running")

	ok := CreateSubnetCommand{Name: "sn-1", VPC: "vpc-1", Mode: "bridge", IfaceType: "vms", InterfaceIP: "10.0.0.1", CIDR: "10.0.0.0/24"}
	if err := ok.Prepare(db, testCfg()); err != nil {
		t.Errorf("bridge br-vms à 1500 : %v", err)
	}
	small := CreateSubnetCommand{Name: "sn-2", VPC: "vpc-1", Mode: "bridge", InterfaceIP: "10.0.1.1", CIDR: "10.0.1.0/24"}
	if err := small.Prepare(db, testCfg()); err == nil || !strings.Contains(err.Error(), "bridge br-default has mtu 1499") {
		t.Errorf("bridge br-default à 1499 : %v", err)
	}
	if _, err := kv.GetFromDB(db, "subnet/sn-2/state"); err == nil {
		t.Error("aucune clé ne doit être écrite après un refus")
	}
}

func TestCreateVPCCommand_Prepare_L3VNIChecksTheUnderlay(t *testing.T) {
	cfg := bgpConfig()
	cfg.DefaultInterface = "br-000000"
	withLinkMTU(t, map[string]int{"br-000000": 1500})
	_, db := newTestDispatcher(t)

	err := (CreateVPCCommand{Name: "vpc-1", CIDR: "10.0.0.0/16", L3VNI: 4001}).Prepare(db, cfg)
	if err == nil || !strings.Contains(err.Error(), "underlay br-000000") {
		t.Fatalf("underlay br-000000 à 1500 : %v", err)
	}
	if _, err := kv.GetFromDB(db, "vpc/vpc-1/state"); err == nil {
		t.Error("aucune clé ne doit être écrite après un refus")
	}
	if err := (CreateVPCCommand{Name: "vpc-2", CIDR: "10.0.0.0/16"}).Prepare(db, cfg); err != nil {
		t.Errorf("un VPC sans l3vni ne contrôle pas l'underlay : %v", err)
	}
}
