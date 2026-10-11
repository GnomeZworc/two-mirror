package dispatcher

import (
	"strings"
	"testing"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/pkg/db/kv"
)

// --- CreateVPCCommand.Prepare ---

func TestCreateVPCCommand_Prepare_NewVPC(t *testing.T) {
	_, db := newTestDispatcher(t)
	cmd := CreateVPCCommand{Name: "vpc-1", CIDR: "10.0.0.0/16"}
	if err := cmd.Prepare(db, nil); err != nil {
		t.Fatalf("Prepare a échoué : %v", err)
	}
	state, err := kv.GetFromDB(db, "vpc/vpc-1/state")
	if err != nil {
		t.Fatalf("état non écrit en DB : %v", err)
	}
	if state != "creating" {
		t.Errorf("state attendu creating, obtenu %q", state)
	}
	cidr, err := kv.GetFromDB(db, "vpc/vpc-1/cidr")
	if err != nil {
		t.Fatalf("cidr non écrit en DB : %v", err)
	}
	if cidr != "10.0.0.0/16" {
		t.Errorf("cidr attendu 10.0.0.0/16, obtenu %q", cidr)
	}
}

func TestCreateVPCCommand_Prepare_Duplicate(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-exist/state", "running")
	cmd := CreateVPCCommand{Name: "vpc-exist", CIDR: "10.0.0.0/16"}
	if err := cmd.Prepare(db, nil); err == nil {
		t.Error("Prepare devrait échouer sur un VPC déjà existant")
	}
}

func TestCreateVPCCommand_Prepare_InvalidCIDR(t *testing.T) {
	_, db := newTestDispatcher(t)
	cmd := CreateVPCCommand{Name: "vpc-bad", CIDR: "not-a-cidr"}
	if err := cmd.Prepare(db, nil); err == nil {
		t.Error("Prepare devrait échouer avec un CIDR invalide")
	}
}

// --- DeleteVPCCommand.Prepare ---

func TestDeleteVPCCommand_Prepare_Success(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-del/state", "running")
	cmd := DeleteVPCCommand{Name: "vpc-del"}
	if err := cmd.Prepare(db, nil); err != nil {
		t.Fatalf("Prepare a échoué : %v", err)
	}
	state, _ := kv.GetFromDB(db, "vpc/vpc-del/state")
	if state != "deleting" {
		t.Errorf("state attendu deleting, obtenu %q", state)
	}
}

func TestDeleteVPCCommand_Prepare_NotFound(t *testing.T) {
	_, db := newTestDispatcher(t)
	cmd := DeleteVPCCommand{Name: "vpc-inexistant"}
	if err := cmd.Prepare(db, nil); err == nil {
		t.Error("Prepare devrait échouer si le VPC n'existe pas")
	}
}

func TestDeleteVPCCommand_Prepare_RefusedWhileCreating(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-wip/state", "creating")
	cmd := DeleteVPCCommand{Name: "vpc-wip"}
	if err := cmd.Prepare(db, nil); err == nil {
		t.Error("Prepare devrait refuser la suppression d'un VPC en creating")
	}
	s, _ := kv.GetFromDB(db, "vpc/vpc-wip/state")
	if s != "creating" {
		t.Errorf("l'état ne devrait pas changer, obtenu %q", s)
	}
}

func TestDeleteVPCCommand_Prepare_RefusedWhileDeleting(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-gone/state", "deleting")
	cmd := DeleteVPCCommand{Name: "vpc-gone"}
	if err := cmd.Prepare(db, nil); err == nil {
		t.Error("Prepare devrait refuser un VPC déjà en deleting")
	}
}

func TestDeleteVPCCommand_Prepare_AllowedFromError(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-ko/state", "error")
	cmd := DeleteVPCCommand{Name: "vpc-ko"}
	if err := cmd.Prepare(db, nil); err != nil {
		t.Fatalf("Prepare devrait accepter un VPC en error : %v", err)
	}
	s, _ := kv.GetFromDB(db, "vpc/vpc-ko/state")
	if s != "deleting" {
		t.Errorf("state attendu deleting, obtenu %q", s)
	}
}

func TestDeleteVPCCommand_Prepare_BlockedByActiveSubnet(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-busy/state", "running")
	kv.AddInDB(db, "subnet/sn-1/state", "running")
	kv.AddInDB(db, "subnet/sn-1/vpc", "vpc-busy")
	cmd := DeleteVPCCommand{Name: "vpc-busy"}
	if err := cmd.Prepare(db, nil); err == nil {
		t.Error("Prepare devrait échouer si un subnet actif existe")
	}
}

func TestDeleteVPCCommand_Prepare_AllowedWhenSubnetDeleted(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-ok/state", "running")
	kv.AddInDB(db, "subnet/sn-1/state", "deleted")
	kv.AddInDB(db, "subnet/sn-1/vpc", "vpc-ok")
	cmd := DeleteVPCCommand{Name: "vpc-ok"}
	if err := cmd.Prepare(db, nil); err != nil {
		t.Fatalf("Prepare devrait réussir si le subnet est deleted : %v", err)
	}
}

func TestDeleteVPCCommand_Prepare_AllowedWhenSubnetDeleting(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-ok/state", "running")
	kv.AddInDB(db, "subnet/sn-1/state", "deleting")
	kv.AddInDB(db, "subnet/sn-1/vpc", "vpc-ok")
	cmd := DeleteVPCCommand{Name: "vpc-ok"}
	if err := cmd.Prepare(db, nil); err != nil {
		t.Fatalf("Prepare devrait réussir si le subnet est deleting : %v", err)
	}
}

func bgpConfig() *configuration.Config {
	cfg := &configuration.Config{}
	cfg.BGP.ASN = 64600
	cfg.BGP.RouteReflectors = []string{"10.255.255.1"}
	return cfg
}

func TestCreateVPCCommand_Prepare_StoresTheL3VNI(t *testing.T) {
	_, db := newTestDispatcher(t)
	cmd := CreateVPCCommand{Name: "vpc-1", CIDR: "10.0.0.0/16", L3VNI: 4001}
	if err := cmd.Prepare(db, bgpConfig()); err != nil {
		t.Fatalf("Prepare a échoué : %v", err)
	}
	if got, err := kv.GetFromDB(db, "vpc/vpc-1/l3vni"); err != nil || got != "4001" {
		t.Errorf("l3vni en DB = %q, %v ; attendu 4001", got, err)
	}
}

func TestCreateVPCCommand_Prepare_WithoutL3VNIStoresNothing(t *testing.T) {
	_, db := newTestDispatcher(t)
	cmd := CreateVPCCommand{Name: "vpc-1", CIDR: "10.0.0.0/16"}
	if err := cmd.Prepare(db, nil); err != nil {
		t.Fatalf("Prepare a échoué : %v", err)
	}
	if got, err := kv.GetFromDB(db, "vpc/vpc-1/l3vni"); err == nil {
		t.Errorf("clé l3vni écrite : %q", got)
	}
}

func TestCreateVPCCommand_Prepare_RefusesAnL3VNIWithoutBGP(t *testing.T) {
	_, db := newTestDispatcher(t)
	cmd := CreateVPCCommand{Name: "vpc-1", CIDR: "10.0.0.0/16", L3VNI: 4001}
	err := cmd.Prepare(db, &configuration.Config{})
	if err == nil || !strings.Contains(err.Error(), "requires the bgp section") {
		t.Fatalf("erreur %v", err)
	}
	if entries, _ := kv.ListByPrefix(db, "vpc/"); len(entries) != 0 {
		t.Errorf("écritures malgré le refus : %v", entries)
	}
}

func TestCreateVPCCommand_Prepare_RefusesAReservedL3VNI(t *testing.T) {
	_, db := newTestDispatcher(t)
	cmd := CreateVPCCommand{Name: "vpc-1", CIDR: "10.0.0.0/16", L3VNI: 254}
	if err := cmd.Prepare(db, bgpConfig()); err == nil || !strings.Contains(err.Error(), "l3vni 254 is reserved") {
		t.Fatalf("erreur %v", err)
	}
	if entries, _ := kv.ListByPrefix(db, "vpc/"); len(entries) != 0 {
		t.Errorf("écritures malgré le refus : %v", entries)
	}
}

func TestCreateVPCCommand_Prepare_RefusesAnL3VNIAlreadyUsed(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-a/state", "deleting")
	kv.AddInDB(db, "vpc/vpc-a/l3vni", "4001")
	cmd := CreateVPCCommand{Name: "vpc-b", CIDR: "10.0.0.0/16", L3VNI: 4001}
	err := cmd.Prepare(db, bgpConfig())
	if err == nil || !strings.Contains(err.Error(), `l3vni 4001 is already used by vpc "vpc-a"`) {
		t.Fatalf("erreur %v", err)
	}
	if _, err := kv.GetFromDB(db, "vpc/vpc-b/state"); err == nil {
		t.Error("vpc-b créé malgré le doublon")
	}
}

func TestCreateVPCCommand_Prepare_ReusesTheL3VNIOfADeletedVPC(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-a/state", "deleted")
	kv.AddInDB(db, "vpc/vpc-a/l3vni", "4001")
	kv.AddInDB(db, "vpc/vpc-c/state", "running")
	kv.AddInDB(db, "vpc/vpc-c/l3vni", "4002")
	cmd := CreateVPCCommand{Name: "vpc-b", CIDR: "10.0.0.0/16", L3VNI: 4001}
	if err := cmd.Prepare(db, bgpConfig()); err != nil {
		t.Fatalf("Prepare a échoué : %v", err)
	}
}
