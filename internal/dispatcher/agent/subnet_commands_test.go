package dispatcher

import (
	"testing"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/pkg/db/kv"
)

func testCfg() *configuration.Config {
	cfg := &configuration.Config{DefaultInterface: "br-default"}
	cfg.Interfaces = map[string]string{"vms": "br-vms"}
	return cfg
}

// --- CreateSubnetCommand.Prepare ---

func TestCreateSubnetCommand_Prepare_Success(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "created")
	cmd := CreateSubnetCommand{
		Name: "sn-1", VPC: "vpc-1", VxlanID: 100,
		IfaceType: "vms", GatewayIP: "10.0.0.1", CIDR: "10.0.0.0/24",
	}
	if err := cmd.Prepare(db, testCfg()); err != nil {
		t.Fatalf("Prepare a échoué : %v", err)
	}
	state, _ := kv.GetFromDB(db, "subnet/sn-1/state")
	if state != "creating" {
		t.Errorf("state attendu creating, obtenu %q", state)
	}
	vpc, _ := kv.GetFromDB(db, "subnet/sn-1/vpc")
	if vpc != "vpc-1" {
		t.Errorf("vpc attendu vpc-1, obtenu %q", vpc)
	}
}

func TestCreateSubnetCommand_Prepare_UsesIfaceTypeMapping(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "created")
	cmd := CreateSubnetCommand{
		Name: "sn-1", VPC: "vpc-1", VxlanID: 100,
		IfaceType: "vms", GatewayIP: "10.0.0.1", CIDR: "10.0.0.0/24",
	}
	cmd.Prepare(db, testCfg())
	iface, _ := kv.GetFromDB(db, "subnet/sn-1/local_iface")
	if iface != "br-vms" {
		t.Errorf("local_iface attendu br-vms, obtenu %q", iface)
	}
}

func TestCreateSubnetCommand_Prepare_UsesDefaultIfaceWhenTypeUnknown(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "created")
	cmd := CreateSubnetCommand{
		Name: "sn-1", VPC: "vpc-1", VxlanID: 100,
		IfaceType: "inconnu", GatewayIP: "10.0.0.1", CIDR: "10.0.0.0/24",
	}
	cmd.Prepare(db, testCfg())
	iface, _ := kv.GetFromDB(db, "subnet/sn-1/local_iface")
	if iface != "br-default" {
		t.Errorf("local_iface attendu br-default, obtenu %q", iface)
	}
}

func TestCreateSubnetCommand_Prepare_Duplicate(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "created")
	kv.AddInDB(db, "subnet/sn-exist/state", "created")
	cmd := CreateSubnetCommand{
		Name: "sn-exist", VPC: "vpc-1", VxlanID: 100,
		IfaceType: "vms", GatewayIP: "10.0.0.1", CIDR: "10.0.0.0/24",
	}
	if err := cmd.Prepare(db, testCfg()); err == nil {
		t.Error("Prepare devrait échouer sur un subnet déjà existant")
	}
}

func TestCreateSubnetCommand_Prepare_VPCNotFound(t *testing.T) {
	_, db := newTestDispatcher(t)
	cmd := CreateSubnetCommand{
		Name: "sn-1", VPC: "vpc-inexistant", VxlanID: 100,
		IfaceType: "vms", GatewayIP: "10.0.0.1", CIDR: "10.0.0.0/24",
	}
	if err := cmd.Prepare(db, testCfg()); err == nil {
		t.Error("Prepare devrait échouer si le VPC n'existe pas")
	}
}

func TestCreateSubnetCommand_Prepare_VPCDeleting(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-dying/state", "deleting")
	cmd := CreateSubnetCommand{
		Name: "sn-1", VPC: "vpc-dying", VxlanID: 100,
		IfaceType: "vms", GatewayIP: "10.0.0.1", CIDR: "10.0.0.0/24",
	}
	if err := cmd.Prepare(db, testCfg()); err == nil {
		t.Error("Prepare devrait échouer si le VPC est en cours de suppression")
	}
}

func TestCreateSubnetCommand_Prepare_VPCDeleted(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-gone/state", "deleted")
	cmd := CreateSubnetCommand{
		Name: "sn-1", VPC: "vpc-gone", VxlanID: 100,
		IfaceType: "vms", GatewayIP: "10.0.0.1", CIDR: "10.0.0.0/24",
	}
	if err := cmd.Prepare(db, testCfg()); err == nil {
		t.Error("Prepare devrait échouer si le VPC est supprimé")
	}
}

// --- DeleteSubnetCommand.Prepare ---

func TestDeleteSubnetCommand_Prepare_Success(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "subnet/sn-del/state", "created")
	cmd := DeleteSubnetCommand{Name: "sn-del"}
	if err := cmd.Prepare(db, nil); err != nil {
		t.Fatalf("Prepare a échoué : %v", err)
	}
	state, _ := kv.GetFromDB(db, "subnet/sn-del/state")
	if state != "deleting" {
		t.Errorf("state attendu deleting, obtenu %q", state)
	}
}

func TestDeleteSubnetCommand_Prepare_NotFound(t *testing.T) {
	_, db := newTestDispatcher(t)
	cmd := DeleteSubnetCommand{Name: "sn-inexistant"}
	if err := cmd.Prepare(db, nil); err == nil {
		t.Error("Prepare devrait échouer si le subnet n'existe pas")
	}
}
