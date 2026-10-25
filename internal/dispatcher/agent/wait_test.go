package dispatcher

import (
	"errors"
	"strings"
	"testing"
	"time"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/pkg/db/kv"
)

func TestWaitUntil(t *testing.T) {
	calls := 0
	err := waitUntil(time.Second, time.Millisecond, func() (bool, error) {
		calls++
		return calls == 3, nil
	})
	if err != nil || calls != 3 {
		t.Errorf("err %v, %d appels, attendu fin au troisième", err, calls)
	}

	if err := waitUntil(30*time.Millisecond, 5*time.Millisecond, func() (bool, error) { return false, nil }); !errors.Is(err, errWaitTimeout) {
		t.Errorf("délai dépassé : %v", err)
	}

	boom := errors.New("boom")
	calls = 0
	err = waitUntil(time.Second, time.Millisecond, func() (bool, error) {
		calls++
		return false, boom
	})
	if !errors.Is(err, boom) || calls != 1 {
		t.Errorf("erreur %v après %d appels, attendu boom au premier", err, calls)
	}
}

func TestDeleteSubnetCommand_Prepare_AcceptsVMsBeingDeleted(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "subnet/sn-a/state", "running")
	kv.AddInDB(db, "vm/i-a/state", "deleting")
	kv.AddInDB(db, "vm/i-a/nic/0/subnet", "sn-a")
	kv.AddInDB(db, "vm/i-gone/state", "deleted")
	kv.AddInDB(db, "vm/i-gone/nic/0/subnet", "sn-a")
	if err := (DeleteSubnetCommand{Name: "sn-a"}).Prepare(db, nil); err != nil {
		t.Fatalf("VM en deleting : %v", err)
	}
	if s, _ := kv.GetFromDB(db, "subnet/sn-a/state"); s != "deleting" {
		t.Errorf("état %q, attendu deleting", s)
	}
}

func TestDeleteSubnetCommand_Prepare_ListsOnlyTheVMsNotBeingDeleted(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "subnet/sn-a/state", "running")
	kv.AddInDB(db, "vm/i-a/state", "deleting")
	kv.AddInDB(db, "vm/i-a/nic/0/subnet", "sn-a")
	kv.AddInDB(db, "vm/i-b/state", "running")
	kv.AddInDB(db, "vm/i-b/nic/0/subnet", "sn-a")
	err := DeleteSubnetCommand{Name: "sn-a"}.Prepare(db, nil)
	if err == nil || !strings.Contains(err.Error(), `subnet "sn-a" is still used by vms i-b`) || strings.Contains(err.Error(), "i-a") {
		t.Fatalf("erreur %v", err)
	}
}

func TestWaitVMsDeleted_ReturnsOnceTheVMsAreGone(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vm/i-a/state", "deleting")
	kv.AddInDB(db, "vm/i-a/nic/0/subnet", "sn-a")
	kv.AddInDB(db, "vm/i-other/state", "running")
	kv.AddInDB(db, "vm/i-other/nic/0/subnet", "sn-b")
	go func() {
		time.Sleep(30 * time.Millisecond)
		kv.DeleteInDB(db, "vm/i-a")
	}()
	if err := waitVMsDeleted(db, "sn-a", 2*time.Second, 5*time.Millisecond); err != nil {
		t.Fatalf("attente : %v", err)
	}
}

func TestWaitVMsDeleted_FailsWhenAVMEndsInError(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vm/i-a/state", "deleting")
	kv.AddInDB(db, "vm/i-a/nic/0/subnet", "sn-a")
	go func() {
		time.Sleep(20 * time.Millisecond)
		kv.AddInDB(db, "vm/i-a/state", "error")
	}()
	err := waitVMsDeleted(db, "sn-a", 2*time.Second, 5*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), `vm "i-a" is error, subnet "sn-a" cannot be deleted`) {
		t.Fatalf("erreur %v", err)
	}
}

func TestWaitVMsDeleted_TimesOut(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vm/i-a/state", "deleting")
	kv.AddInDB(db, "vm/i-a/nic/0/subnet", "sn-a")
	err := waitVMsDeleted(db, "sn-a", 30*time.Millisecond, 5*time.Millisecond)
	if err == nil || err.Error() != `timed out waiting for the vms of subnet "sn-a" to be deleted` {
		t.Fatalf("erreur %v", err)
	}
}

func TestWaitSubnetsDeleted_ReturnsOnceTheSubnetsAreGone(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "subnet/sn-a/state", "deleting")
	kv.AddInDB(db, "subnet/sn-a/vpc", "vp-a")
	kv.AddInDB(db, "subnet/sn-b/state", "deleted")
	kv.AddInDB(db, "subnet/sn-b/vpc", "vp-a")
	kv.AddInDB(db, "subnet/sn-x/state", "running")
	kv.AddInDB(db, "subnet/sn-x/vpc", "vp-x")
	go func() {
		time.Sleep(30 * time.Millisecond)
		kv.DeleteInDB(db, "subnet/sn-a")
	}()
	if err := waitSubnetsDeleted(db, "vp-a", 2*time.Second, 5*time.Millisecond); err != nil {
		t.Fatalf("attente : %v", err)
	}
}

func TestWaitSubnetsDeleted_FailsWhenASubnetEndsInError(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "subnet/sn-a/state", "deleting")
	kv.AddInDB(db, "subnet/sn-a/vpc", "vp-a")
	go func() {
		time.Sleep(20 * time.Millisecond)
		kv.AddInDB(db, "subnet/sn-a/state", "error")
	}()
	err := waitSubnetsDeleted(db, "vp-a", 2*time.Second, 5*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), `subnet "sn-a" is error, vpc "vp-a" cannot be deleted`) {
		t.Fatalf("erreur %v", err)
	}
}

func TestWaitSubnetsDeleted_TimesOut(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "subnet/sn-a/state", "deleting")
	kv.AddInDB(db, "subnet/sn-a/vpc", "vp-a")
	err := waitSubnetsDeleted(db, "vp-a", 30*time.Millisecond, 5*time.Millisecond)
	if err == nil || err.Error() != `timed out waiting for subnets of vpc "vp-a" to be deleted` {
		t.Fatalf("erreur %v", err)
	}
}

func TestDeleteSubnetCommand_Execute_FailsBeforeTouchingTheHostWhenAVMIsInError(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "subnet/sn-a/state", "deleting")
	kv.AddInDB(db, "vm/i-a/state", "error")
	kv.AddInDB(db, "vm/i-a/nic/0/subnet", "sn-a")
	cfg := &configuration.Config{Times: configuration.Times{TimeoutSubnetStop: 1, PollInterval: 1}}
	err := DeleteSubnetCommand{Name: "sn-a"}.Execute(db, cfg)
	if err == nil || err.Error() != `vm "i-a" is error, subnet "sn-a" cannot be deleted` {
		t.Fatalf("erreur %v", err)
	}
}

func TestDeleteVPCCommand_Execute_FailsBeforeTouchingTheHostWhenASubnetIsInError(t *testing.T) {
	_, db := newTestDispatcher(t)
	kv.AddInDB(db, "vpc/vp-a/state", "deleting")
	kv.AddInDB(db, "subnet/sn-a/state", "error")
	kv.AddInDB(db, "subnet/sn-a/vpc", "vp-a")
	cfg := &configuration.Config{Times: configuration.Times{TimeoutVPCStop: 1, PollInterval: 1}}
	err := DeleteVPCCommand{Name: "vp-a"}.Execute(db, cfg)
	if err == nil || err.Error() != `subnet "sn-a" is error, vpc "vp-a" cannot be deleted` {
		t.Fatalf("erreur %v", err)
	}
}
