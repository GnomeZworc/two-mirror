package dispatcher

import (
	"strings"
	"testing"

	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
)

func seedVMWithInterfaces(t *testing.T, db *badger.DB, name, st string) {
	t.Helper()
	for key, value := range map[string]string{
		"vpc/vp-t47/cidr":              "10.47.0.0/16",
		"subnet/sn-a/mode":             "vxlan",
		"subnet/sn-p/vpc":              "vp-t47",
		"subnet/sn-p/mode":             "public_ip",
		"vm/" + name + "/nic/2/name":   "nic-pub",
		"vm/" + name + "/nic/2/subnet": "sn-p",
		"vm/" + name + "/nic/2/ip":     "203.0.113.20",
		"subnet/sn-a/vpc":              "vp-t47",
		"vm/" + name + "/state":        st,
		"vm/" + name + "/nic/0/name":   "nic-a",
		"vm/" + name + "/nic/0/subnet": "sn-a",
		"vm/" + name + "/nic/0/ip":     "10.47.1.10",
		"vm/" + name + "/nic/1/name":   "nic-ab",
		"vm/" + name + "/nic/1/subnet": "sn-a",
		"vm/" + name + "/nic/1/ip":     "10.47.1.11",
	} {
		kv.AddInDB(db, key, value)
	}
}

func validFloatingIP() CreateFloatingIPCommand {
	return CreateFloatingIPCommand{Name: "fip-1", VM: "vm-1", Interface: "nic-a", IP: "240.0.0.47/24", Gateway: "240.0.0.1"}
}

func TestCreateFloatingIPCommand_Prepare_WritesTheFloatingIP(t *testing.T) {
	_, db := newTestDispatcher(t)
	seedVMWithInterfaces(t, db, "vm-1", "running")
	if err := validFloatingIP().Prepare(db, nil); err != nil {
		t.Fatalf("Prepare a échoué : %v", err)
	}
	for key, want := range map[string]string{
		"floating-ip/fip-1/state":     "creating",
		"floating-ip/fip-1/vm":        "vm-1",
		"floating-ip/fip-1/interface": "nic-a",
		"floating-ip/fip-1/ip":        "240.0.0.47/24",
		"floating-ip/fip-1/gateway":   "240.0.0.1",
	} {
		if got, err := kv.GetFromDB(db, key); err != nil || got != want {
			t.Errorf("%s : %q (%v), %q attendu", key, got, err, want)
		}
	}
}

func TestCreateFloatingIPCommand_Prepare_Refuses(t *testing.T) {
	cases := []struct {
		label  string
		seed   func(*badger.DB)
		mutate func(*CreateFloatingIPCommand)
		msg    string
	}{
		{"IP flottante existante", func(db *badger.DB) { kv.AddInDB(db, "floating-ip/fip-1/state", "running") }, nil, `floating ip "fip-1" already exists`},
		{"VM absente", nil, func(c *CreateFloatingIPCommand) { c.VM = "vm-x" }, `vm "vm-x" not found`},
		{"VM en création", func(db *badger.DB) { seedVMWithInterfaces(t, db, "vm-2", "creating") }, func(c *CreateFloatingIPCommand) { c.VM = "vm-2" }, `vm "vm-2" is creating`},
		{"interface inconnue", nil, func(c *CreateFloatingIPCommand) { c.Interface = "nic-z" }, `vm "vm-1" has no interface "nic-z"`},
		{"interface qui a déjà une IP flottante", func(db *badger.DB) {
			kv.AddInDB(db, "floating-ip/fip-0/state", "error")
			kv.AddInDB(db, "floating-ip/fip-0/vm", "vm-1")
			kv.AddInDB(db, "floating-ip/fip-0/interface", "nic-a")
			kv.AddInDB(db, "floating-ip/fip-0/ip", "240.0.0.40/24")
		}, nil, `interface "nic-a" of vm "vm-1" already has floating ip "fip-0"`},
		{"IP publique déjà portée", func(db *badger.DB) {
			kv.AddInDB(db, "floating-ip/fip-0/state", "running")
			kv.AddInDB(db, "floating-ip/fip-0/vm", "vm-1")
			kv.AddInDB(db, "floating-ip/fip-0/interface", "nic-ab")
			kv.AddInDB(db, "floating-ip/fip-0/ip", "240.0.0.47/24")
		}, nil, `public ip 240.0.0.47 is already used by floating ip "fip-0"`},
		{"interface sur un subnet public_ip", nil, func(c *CreateFloatingIPCommand) { c.Interface = "nic-pub" }, `interface "nic-pub" of vm "vm-1" is on public_ip subnet "sn-p"`},
		{"ip sans préfixe", nil, func(c *CreateFloatingIPCommand) { c.IP = "240.0.0.47" }, `invalid ip "240.0.0.47"`},
		{"gateway invalide", nil, func(c *CreateFloatingIPCommand) { c.Gateway = "bordure" }, `invalid gateway "bordure"`},
	}
	for _, c := range cases {
		_, db := newTestDispatcher(t)
		seedVMWithInterfaces(t, db, "vm-1", "running")
		if c.seed != nil {
			c.seed(db)
		}
		cmd := validFloatingIP()
		if c.mutate != nil {
			c.mutate(&cmd)
		}
		err := cmd.Prepare(db, nil)
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s : erreur %v, « %s » attendu", c.label, err, c.msg)
			continue
		}
		if c.label != "IP flottante existante" {
			if _, err := kv.GetFromDB(db, "floating-ip/fip-1/state"); err == nil {
				t.Errorf("%s : état écrit malgré le refus", c.label)
			}
		}
	}
}

func TestCreateFloatingIPCommand_Prepare_IgnoresNearbyNames(t *testing.T) {
	_, db := newTestDispatcher(t)
	seedVMWithInterfaces(t, db, "vm-1", "running")
	seedVMWithInterfaces(t, db, "vm-10", "running")
	kv.AddInDB(db, "floating-ip/fip-0/state", "running")
	kv.AddInDB(db, "floating-ip/fip-0/vm", "vm-10")
	kv.AddInDB(db, "floating-ip/fip-0/interface", "nic-a")
	kv.AddInDB(db, "floating-ip/fip-0/ip", "240.0.0.40/24")
	kv.AddInDB(db, "floating-ip/fip-2/state", "running")
	kv.AddInDB(db, "floating-ip/fip-2/vm", "vm-1")
	kv.AddInDB(db, "floating-ip/fip-2/interface", "nic-ab")
	kv.AddInDB(db, "floating-ip/fip-2/ip", "240.0.0.41/24")
	if err := validFloatingIP().Prepare(db, nil); err != nil {
		t.Errorf("Prepare a échoué : %v", err)
	}
}

func TestDeleteFloatingIPCommand_Prepare(t *testing.T) {
	for st, ok := range map[string]bool{"running": true, "error": true, "creating": false, "deleting": false} {
		_, db := newTestDispatcher(t)
		kv.AddInDB(db, "floating-ip/fip-1/state", st)
		err := DeleteFloatingIPCommand{Name: "fip-1"}.Prepare(db, nil)
		if ok != (err == nil) {
			t.Errorf("état %s : erreur %v", st, err)
		}
	}
	_, db := newTestDispatcher(t)
	if err := (DeleteFloatingIPCommand{Name: "fip-x"}).Prepare(db, nil); err == nil || !strings.Contains(err.Error(), `floating ip "fip-x" not found`) {
		t.Errorf("IP flottante absente : %v", err)
	}
}

func TestStopVMCommand_Prepare_RefusesWhileAFloatingIPIsAttached(t *testing.T) {
	for _, st := range []string{"running", "error", "creating", "deleting"} {
		_, db := newTestDispatcher(t)
		seedVMWithInterfaces(t, db, "vm-1", "running")
		kv.AddInDB(db, "floating-ip/fip-1/state", st)
		kv.AddInDB(db, "floating-ip/fip-1/vm", "vm-1")
		kv.AddInDB(db, "floating-ip/fip-1/interface", "nic-a")
		err := StopVMCommand{Name: "vm-1"}.Prepare(db, nil)
		if err == nil || !strings.Contains(err.Error(), `floating ip "fip-1" must be deleted before deleting vm "vm-1"`) {
			t.Errorf("IP flottante %s : erreur %v", st, err)
		}
		if got, _ := kv.GetFromDB(db, "vm/vm-1/state"); got != "running" {
			t.Errorf("IP flottante %s : VM passée en %q malgré le refus", st, got)
		}
	}
}

func TestStopVMCommand_Prepare_IgnoresTheFloatingIPOfANearbyVM(t *testing.T) {
	_, db := newTestDispatcher(t)
	seedVMWithInterfaces(t, db, "vm-1", "running")
	kv.AddInDB(db, "floating-ip/fip-1/state", "running")
	kv.AddInDB(db, "floating-ip/fip-1/vm", "vm-10")
	if err := (StopVMCommand{Name: "vm-1"}).Prepare(db, nil); err != nil {
		t.Errorf("Prepare a échoué : %v", err)
	}
}
