package dispatcher

import (
	"testing"

	"git.g3e.fr/syonad/two/pkg/db/kv"
)

// --- CreateVPCCommand.Prepare ---

func TestCreateVPCCommand_Prepare_NewVPC(t *testing.T) {
	_, db := newTestDispatcher(t)
	cmd := CreateVPCCommand{Name: "vpc-1"}
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
}

func TestCreateVPCCommand_Prepare_Duplicate(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-exist/state", "created")
	cmd := CreateVPCCommand{Name: "vpc-exist"}
	if err := cmd.Prepare(db, nil); err == nil {
		t.Error("Prepare devrait échouer sur un VPC déjà existant")
	}
}

// --- DeleteVPCCommand.Prepare ---

func TestDeleteVPCCommand_Prepare_Success(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-del/state", "created")
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

func TestDeleteVPCCommand_Prepare_BlockedByActiveSubnet(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-busy/state", "created")
	kv.AddInDB(db, "subnet/sn-1/state", "created")
	kv.AddInDB(db, "subnet/sn-1/vpc", "vpc-busy")
	cmd := DeleteVPCCommand{Name: "vpc-busy"}
	if err := cmd.Prepare(db, nil); err == nil {
		t.Error("Prepare devrait échouer si un subnet actif existe")
	}
}

func TestDeleteVPCCommand_Prepare_AllowedWhenSubnetDeleted(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-ok/state", "created")
	kv.AddInDB(db, "subnet/sn-1/state", "deleted")
	kv.AddInDB(db, "subnet/sn-1/vpc", "vpc-ok")
	cmd := DeleteVPCCommand{Name: "vpc-ok"}
	if err := cmd.Prepare(db, nil); err != nil {
		t.Fatalf("Prepare devrait réussir si le subnet est deleted : %v", err)
	}
}

func TestDeleteVPCCommand_Prepare_AllowedWhenSubnetDeleting(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vpc-ok/state", "created")
	kv.AddInDB(db, "subnet/sn-1/state", "deleting")
	kv.AddInDB(db, "subnet/sn-1/vpc", "vpc-ok")
	cmd := DeleteVPCCommand{Name: "vpc-ok"}
	if err := cmd.Prepare(db, nil); err != nil {
		t.Fatalf("Prepare devrait réussir si le subnet est deleting : %v", err)
	}
}
