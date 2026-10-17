package dispatcher

import (
	"strings"
	"testing"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
)

func testCfg() *configuration.Config {
	cfg := &configuration.Config{DefaultInterface: "br-default"}
	cfg.Interfaces = map[string]string{"vms": "br-vms"}
	return cfg
}

// --- CreateSubnetCommand.Prepare ---

func TestCreateSubnetCommand_Prepare_Success(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "running")
	cmd := CreateSubnetCommand{
		Name: "sn-1", VPC: "vpc-1", VxlanID: 100,
		IfaceType: "vms", CIDR: "10.0.0.0/24",
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
	ip, _ := kv.GetFromDB(db, "subnet/sn-1/interface_ip")
	if ip != "10.0.0.254" {
		t.Errorf("interface_ip attendue 10.0.0.254, obtenu %q", ip)
	}
}

func TestCreateSubnetCommand_Prepare_UsesIfaceTypeMapping(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "running")
	cmd := CreateSubnetCommand{
		Name: "sn-1", VPC: "vpc-1", VxlanID: 100,
		IfaceType: "vms", CIDR: "10.0.0.0/24",
	}
	cmd.Prepare(db, testCfg())
	iface, _ := kv.GetFromDB(db, "subnet/sn-1/local_iface")
	if iface != "br-vms" {
		t.Errorf("local_iface attendu br-vms, obtenu %q", iface)
	}
}

func TestCreateSubnetCommand_Prepare_UsesDefaultIfaceWhenTypeUnknown(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "running")
	cmd := CreateSubnetCommand{
		Name: "sn-1", VPC: "vpc-1", VxlanID: 100,
		IfaceType: "inconnu", CIDR: "10.0.0.0/24",
	}
	cmd.Prepare(db, testCfg())
	iface, _ := kv.GetFromDB(db, "subnet/sn-1/local_iface")
	if iface != "br-default" {
		t.Errorf("local_iface attendu br-default, obtenu %q", iface)
	}
}

func TestCreateSubnetCommand_Prepare_Duplicate(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "running")
	kv.AddInDB(db, "subnet/sn-exist/state", "running")
	cmd := CreateSubnetCommand{
		Name: "sn-exist", VPC: "vpc-1", VxlanID: 100,
		IfaceType: "vms", CIDR: "10.0.0.0/24",
	}
	if err := cmd.Prepare(db, testCfg()); err == nil {
		t.Error("Prepare devrait échouer sur un subnet déjà existant")
	}
}

func TestCreateSubnetCommand_Prepare_VPCNotFound(t *testing.T) {
	_, db := newTestDispatcher(t)
	cmd := CreateSubnetCommand{
		Name: "sn-1", VPC: "vpc-inexistant", VxlanID: 100,
		IfaceType: "vms", CIDR: "10.0.0.0/24",
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
		IfaceType: "vms", CIDR: "10.0.0.0/24",
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
		IfaceType: "vms", CIDR: "10.0.0.0/24",
	}
	if err := cmd.Prepare(db, testCfg()); err == nil {
		t.Error("Prepare devrait échouer si le VPC est supprimé")
	}
}

func TestCreateSubnetCommand_Prepare_DefaultsToVxlanMode(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "running")
	cmd := CreateSubnetCommand{
		Name: "sn-1", VPC: "vpc-1", VxlanID: 100,
		IfaceType: "vms", CIDR: "10.0.0.0/24",
	}
	cmd.Prepare(db, testCfg())
	mode, _ := kv.GetFromDB(db, "subnet/sn-1/mode")
	if mode != "vxlan" {
		t.Errorf("mode attendu vxlan, obtenu %q", mode)
	}
	if _, err := kv.GetFromDB(db, "subnet/sn-1/vxlan_id"); err != nil {
		t.Error("vxlan_id devrait être écrit en mode vxlan")
	}
}

func TestCreateSubnetCommand_Prepare_BridgeMode_Success(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "running")
	cmd := CreateSubnetCommand{
		Name: "sn-1", VPC: "vpc-1", Mode: "bridge",
		IfaceType: "vms", InterfaceIP: "10.0.0.1", CIDR: "10.0.0.0/24",
	}
	if err := cmd.Prepare(db, testCfg()); err != nil {
		t.Fatalf("Prepare a échoué : %v", err)
	}
	mode, _ := kv.GetFromDB(db, "subnet/sn-1/mode")
	if mode != "bridge" {
		t.Errorf("mode attendu bridge, obtenu %q", mode)
	}
	iface, _ := kv.GetFromDB(db, "subnet/sn-1/local_iface")
	if iface != "br-vms" {
		t.Errorf("local_iface attendu br-vms, obtenu %q", iface)
	}
}

func TestCreateSubnetCommand_Prepare_BridgeMode_NoVxlanID(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "running")
	cmd := CreateSubnetCommand{
		Name: "sn-1", VPC: "vpc-1", Mode: "bridge",
		IfaceType: "vms", InterfaceIP: "10.0.0.1", CIDR: "10.0.0.0/24",
	}
	cmd.Prepare(db, testCfg())
	if _, err := kv.GetFromDB(db, "subnet/sn-1/vxlan_id"); err == nil {
		t.Error("vxlan_id ne devrait pas être écrit en mode bridge")
	}
}

func TestCreateSubnetCommand_Prepare_UnknownMode(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "running")
	cmd := CreateSubnetCommand{
		Name: "sn-1", VPC: "vpc-1", Mode: "vlan",
		IfaceType: "vms", InterfaceIP: "10.0.0.1", CIDR: "10.0.0.0/24",
	}
	if err := cmd.Prepare(db, testCfg()); err == nil {
		t.Error("Prepare devrait échouer pour un mode inconnu")
	}
}

func TestCreateSubnetCommand_Prepare_DefaultRouteStored(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "running")
	cmd := CreateSubnetCommand{
		Name: "sn-1", VPC: "vpc-1", VxlanID: 100,
		IfaceType: "vms", CIDR: "10.0.0.0/24",
		DefaultRoute: true,
	}
	if err := cmd.Prepare(db, testCfg()); err != nil {
		t.Fatalf("Prepare a échoué : %v", err)
	}
	val, err := kv.GetFromDB(db, "subnet/sn-1/default_route")
	if err != nil {
		t.Fatalf("default_route non écrit en DB : %v", err)
	}
	if val != "true" {
		t.Errorf("default_route attendu true, obtenu %q", val)
	}
}

func TestCreateSubnetCommand_Prepare_DefaultRouteFalseByDefault(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "running")
	cmd := CreateSubnetCommand{
		Name: "sn-1", VPC: "vpc-1", VxlanID: 100,
		IfaceType: "vms", CIDR: "10.0.0.0/24",
	}
	cmd.Prepare(db, testCfg())
	val, _ := kv.GetFromDB(db, "subnet/sn-1/default_route")
	if val != "false" {
		t.Errorf("default_route attendu false, obtenu %q", val)
	}
}

func TestCreateSubnetCommand_Prepare_VxlanComputesAnycastAddresses(t *testing.T) {
	db, err := prepareSubnet(t, CreateSubnetCommand{
		Name: "sn-any", VxlanID: 100, IfaceType: "vms", CIDR: "10.1.0.0/16",
	})
	if err != nil {
		t.Fatalf("Prepare a échoué : %v", err)
	}
	if ip, _ := kv.GetFromDB(db, "subnet/sn-any/interface_ip"); ip != "10.1.255.254" {
		t.Errorf("interface_ip attendue 10.1.255.254, obtenu %q", ip)
	}
	if relay, _ := kv.GetFromDB(db, "subnet/sn-any/relay_ip"); relay != "10.1.255.253" {
		t.Errorf("relay_ip attendu 10.1.255.253, obtenu %q", relay)
	}
}

func TestCreateSubnetCommand_Prepare_VxlanRefusesInterfaceIP(t *testing.T) {
	for _, mode := range []string{"", "vxlan"} {
		db, err := prepareSubnet(t, CreateSubnetCommand{
			Name: "sn-ip", Mode: mode, VxlanID: 100, IfaceType: "vms",
			InterfaceIP: "10.0.0.1", CIDR: "10.0.0.0/24",
		})
		if err == nil || !strings.Contains(err.Error(), "interface_ip is computed") {
			t.Errorf("mode %q : une interface_ip fournie doit être refusée, obtenu %v", mode, err)
		}
		if _, err := kv.GetFromDB(db, "subnet/sn-ip/state"); err == nil {
			t.Errorf("mode %q : rien ne doit être écrit après un refus", mode)
		}
	}
}

func TestCreateSubnetCommand_Prepare_VxlanRefusesSlash30(t *testing.T) {
	db, err := prepareSubnet(t, CreateSubnetCommand{
		Name: "sn-small", VxlanID: 100, IfaceType: "vms", CIDR: "10.0.0.0/30",
	})
	if err == nil {
		t.Fatal("un /30 doit être refusé en mode vxlan")
	}
	if _, err := kv.GetFromDB(db, "subnet/sn-small/state"); err == nil {
		t.Error("rien ne doit être écrit après un refus")
	}
}

func TestCreateSubnetCommand_Prepare_BridgeRequiresInterfaceIP(t *testing.T) {
	db, err := prepareSubnet(t, CreateSubnetCommand{
		Name: "sn-br", Mode: "bridge", IfaceType: "vms", CIDR: "10.0.0.0/24",
	})
	if err == nil || !strings.Contains(err.Error(), "interface_ip is required") {
		t.Errorf("une interface_ip absente doit être refusée en mode bridge, obtenu %v", err)
	}
	if _, err := kv.GetFromDB(db, "subnet/sn-br/state"); err == nil {
		t.Error("rien ne doit être écrit après un refus")
	}
}

func TestCreateSubnetCommand_Prepare_BridgeStoresGivenInterfaceIPWithoutRelay(t *testing.T) {
	db, err := prepareSubnet(t, CreateSubnetCommand{
		Name: "sn-br", Mode: "bridge", IfaceType: "vms",
		InterfaceIP: "10.0.0.1", CIDR: "10.0.0.0/24",
	})
	if err != nil {
		t.Fatalf("Prepare a échoué : %v", err)
	}
	if ip, _ := kv.GetFromDB(db, "subnet/sn-br/interface_ip"); ip != "10.0.0.1" {
		t.Errorf("interface_ip attendue 10.0.0.1, obtenu %q", ip)
	}
	if _, err := kv.GetFromDB(db, "subnet/sn-br/relay_ip"); err == nil {
		t.Error("relay_ip ne doit être écrit qu'en mode vxlan")
	}
}

// --- DeleteSubnetCommand.Prepare ---

func TestDeleteSubnetCommand_Prepare_Success(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "subnet/sn-del/state", "running")
	cmd := DeleteSubnetCommand{Name: "sn-del"}
	if err := cmd.Prepare(db, nil); err != nil {
		t.Fatalf("Prepare a échoué : %v", err)
	}
	state, _ := kv.GetFromDB(db, "subnet/sn-del/state")
	if state != "deleting" {
		t.Errorf("state attendu deleting, obtenu %q", state)
	}
}

func TestDeleteSubnetCommand_Prepare_RefusedWhileCreating(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "subnet/sn-wip/state", "creating")
	cmd := DeleteSubnetCommand{Name: "sn-wip"}
	if err := cmd.Prepare(db, nil); err == nil {
		t.Error("Prepare devrait refuser la suppression d'un subnet en creating")
	}
	s, _ := kv.GetFromDB(db, "subnet/sn-wip/state")
	if s != "creating" {
		t.Errorf("l'état ne devrait pas changer, obtenu %q", s)
	}
}

func TestDeleteSubnetCommand_Prepare_AllowedFromError(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "subnet/sn-ko/state", "error")
	cmd := DeleteSubnetCommand{Name: "sn-ko"}
	if err := cmd.Prepare(db, nil); err != nil {
		t.Fatalf("Prepare devrait accepter un subnet en error : %v", err)
	}
	s, _ := kv.GetFromDB(db, "subnet/sn-ko/state")
	if s != "deleting" {
		t.Errorf("state attendu deleting, obtenu %q", s)
	}
}

func TestDeleteSubnetCommand_Prepare_NotFound(t *testing.T) {
	_, db := newTestDispatcher(t)
	cmd := DeleteSubnetCommand{Name: "sn-inexistant"}
	if err := cmd.Prepare(db, nil); err == nil {
		t.Error("Prepare devrait échouer si le subnet n'existe pas")
	}
}

// --- gateway optionnelle et mode public_ip ---

func prepareSubnet(t *testing.T, cmd CreateSubnetCommand) (*badger.DB, error) {
	t.Helper()
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-1/state", "running")
	if cmd.VPC == "" {
		cmd.VPC = "vpc-1"
	}
	return db, cmd.Prepare(db, testCfg())
}

func TestCreateSubnetCommand_Prepare_StoresGateway(t *testing.T) {
	db, err := prepareSubnet(t, CreateSubnetCommand{
		Name: "sn-gw", VxlanID: 100, IfaceType: "vms",
		CIDR:         "10.0.0.0/24",
		DefaultRoute: true, Gateway: "10.0.0.254",
	})
	if err != nil {
		t.Fatalf("Prepare a échoué : %v", err)
	}
	gw, err := kv.GetFromDB(db, "subnet/sn-gw/gateway")
	if err != nil {
		t.Fatalf("clé gateway absente : %v", err)
	}
	if gw != "10.0.0.254" {
		t.Errorf("gateway attendue 10.0.0.254, obtenu %q", gw)
	}
}

func TestCreateSubnetCommand_Prepare_NoGatewayWritesNoKey(t *testing.T) {
	db, err := prepareSubnet(t, CreateSubnetCommand{
		Name: "sn-nogw", VxlanID: 100, IfaceType: "vms",
		CIDR: "10.0.0.0/24",
	})
	if err != nil {
		t.Fatalf("Prepare a échoué : %v", err)
	}
	if _, err := kv.GetFromDB(db, "subnet/sn-nogw/gateway"); err == nil {
		t.Error("aucune gateway fournie, aucune clé ne doit être écrite")
	}
}

func TestCreateSubnetCommand_Prepare_AcceptsPublicIPMode(t *testing.T) {
	db, err := prepareSubnet(t, CreateSubnetCommand{
		Name: "sn-pub", Mode: "public_ip", IfaceType: "vms",
		InterfaceIP: "10.0.0.1", CIDR: "10.0.0.0/24",
		DefaultRoute: true, Gateway: "203.0.113.1",
	})
	if err != nil {
		t.Fatalf("le mode public_ip doit être accepté : %v", err)
	}
	mode, _ := kv.GetFromDB(db, "subnet/sn-pub/mode")
	if mode != "public_ip" {
		t.Errorf("mode attendu public_ip, obtenu %q", mode)
	}
	if _, err := kv.GetFromDB(db, "subnet/sn-pub/vxlan_id"); err == nil {
		t.Error("vxlan_id ne doit être écrit que pour le mode vxlan")
	}
}

func TestCreateSubnetCommand_Prepare_RejectsUnknownMode(t *testing.T) {
	_, err := prepareSubnet(t, CreateSubnetCommand{
		Name: "sn-bad", Mode: "public", IfaceType: "vms",
		InterfaceIP: "10.0.0.1", CIDR: "10.0.0.0/24",
	})
	if err == nil {
		t.Error("un mode inconnu doit être refusé")
	}
}

func TestDeleteSubnetCommand_Prepare_RefusedWhileAVMUsesIt(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "subnet/sn-a/state", "running")
	kv.AddInDB(db, "vm/i-gone/state", "deleted")
	kv.AddInDB(db, "vm/i-gone/nic/0/subnet", "sn-a")
	kv.AddInDB(db, "vm/i-other/state", "running")
	kv.AddInDB(db, "vm/i-other/nic/0/subnet", "sn-b")
	kv.AddInDB(db, "vm/i-b/state", "deleting")
	kv.AddInDB(db, "vm/i-b/nic/0/subnet", "sn-b")
	kv.AddInDB(db, "vm/i-b/nic/1/subnet", "sn-a")
	kv.AddInDB(db, "vm/i-a/state", "error")
	kv.AddInDB(db, "vm/i-a/nic/0/subnet", "sn-a")
	kv.AddInDB(db, "vm/i-c/state", "running")
	kv.AddInDB(db, "vm/i-c/nic/0/subnet", "sn-a")
	err := DeleteSubnetCommand{Name: "sn-a"}.Prepare(db, nil)
	if err == nil || !strings.Contains(err.Error(), `subnet "sn-a" is still used by vms i-a, i-b, i-c`) {
		t.Fatalf("erreur %v", err)
	}
	if s, _ := kv.GetFromDB(db, "subnet/sn-a/state"); s != "running" {
		t.Errorf("état modifié malgré le refus : %q", s)
	}
}

func TestDeleteSubnetCommand_Prepare_AllowedOnceItsVMsAreDeleted(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "subnet/sn-a/state", "running")
	kv.AddInDB(db, "vm/i-gone/state", "deleted")
	kv.AddInDB(db, "vm/i-gone/nic/0/subnet", "sn-a")
	kv.AddInDB(db, "vm/i-other/state", "running")
	kv.AddInDB(db, "vm/i-other/nic/0/subnet", "sn-b")
	if err := (DeleteSubnetCommand{Name: "sn-a"}).Prepare(db, nil); err != nil {
		t.Fatalf("Prepare a échoué : %v", err)
	}
}
