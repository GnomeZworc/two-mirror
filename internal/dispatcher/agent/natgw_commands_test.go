package dispatcher

import (
	"strings"
	"testing"

	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
)

func seedRoutedVPC(t *testing.T, db *badger.DB, name, st string) {
	t.Helper()
	kv.AddInDB(db, "vpc/"+name+"/state", st)
	kv.AddInDB(db, "vpc/"+name+"/cidr", "10.241.0.0/16")
	kv.AddInDB(db, "vpc/"+name+"/l3vni", "2700")
}

func validNATGateway() CreateNATGatewayCommand {
	return CreateNATGatewayCommand{Name: "ng-1", VPC: "vp-s7", IP: "198.51.100.50/24", Gateway: "198.51.100.1"}
}

func TestCreateNATGatewayCommand_Prepare_WritesTheGateway(t *testing.T) {
	_, db := newTestDispatcher(t)
	seedRoutedVPC(t, db, "vp-s7", "running")
	if err := validNATGateway().Prepare(db, nil); err != nil {
		t.Fatalf("Prepare a échoué : %v", err)
	}
	for key, want := range map[string]string{
		"nat-gateway/ng-1/state":   "creating",
		"nat-gateway/ng-1/vpc":     "vp-s7",
		"nat-gateway/ng-1/ip":      "198.51.100.50/24",
		"nat-gateway/ng-1/gateway": "198.51.100.1",
	} {
		got, err := kv.GetFromDB(db, key)
		if err != nil || got != want {
			t.Errorf("%s : %q (%v), %q attendu", key, got, err, want)
		}
	}
}

func TestCreateNATGatewayCommand_Prepare_Refuses(t *testing.T) {
	cases := []struct {
		label  string
		seed   func(*badger.DB)
		mutate func(*CreateNATGatewayCommand)
		msg    string
	}{
		{"passerelle existante", func(db *badger.DB) { kv.AddInDB(db, "nat-gateway/ng-1/state", "running") }, nil, `nat gateway "ng-1" already exists`},
		{"VPC absent", nil, func(c *CreateNATGatewayCommand) { c.VPC = "vp-absent" }, `vpc "vp-absent" not found`},
		{"VPC en création", func(db *badger.DB) { seedRoutedVPC(t, db, "vp-new", "creating") }, func(c *CreateNATGatewayCommand) { c.VPC = "vp-new" }, `vpc "vp-new" is creating`},
		{"VPC sans l3vni", func(db *badger.DB) {
			kv.AddInDB(db, "vpc/vp-flat/state", "running")
			kv.AddInDB(db, "vpc/vp-flat/cidr", "10.242.0.0/16")
		}, func(c *CreateNATGatewayCommand) { c.VPC = "vp-flat" }, `vpc "vp-flat" has no l3vni`},
		{"VPC qui a déjà une passerelle", func(db *badger.DB) {
			kv.AddInDB(db, "nat-gateway/ng-0/state", "error")
			kv.AddInDB(db, "nat-gateway/ng-0/vpc", "vp-s7")
		}, nil, `vpc "vp-s7" already has nat gateway "ng-0"`},
		{"ip sans longueur de préfixe", nil, func(c *CreateNATGatewayCommand) { c.IP = "198.51.100.50" }, `invalid ip "198.51.100.50"`},
		{"ip IPv6", nil, func(c *CreateNATGatewayCommand) { c.IP = "2001:db8::50/64" }, `invalid ip "2001:db8::50/64"`},
		{"gateway invalide", nil, func(c *CreateNATGatewayCommand) { c.Gateway = "bordure" }, `invalid gateway "bordure"`},
	}
	for _, c := range cases {
		_, db := newTestDispatcher(t)
		seedRoutedVPC(t, db, "vp-s7", "running")
		if c.seed != nil {
			c.seed(db)
		}
		cmd := validNATGateway()
		if c.mutate != nil {
			c.mutate(&cmd)
		}
		err := cmd.Prepare(db, nil)
		if err == nil || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%s : erreur %v, « %s » attendu", c.label, err, c.msg)
			continue
		}
		if c.label != "passerelle existante" {
			if _, err := kv.GetFromDB(db, "nat-gateway/ng-1/state"); err == nil {
				t.Errorf("%s : état écrit malgré le refus", c.label)
			}
		}
	}
}

func TestCreateNATGatewayCommand_Prepare_IgnoresTheGatewayOfAnotherVPC(t *testing.T) {
	_, db := newTestDispatcher(t)
	seedRoutedVPC(t, db, "vp-s7", "running")
	kv.AddInDB(db, "nat-gateway/ng-0/state", "running")
	kv.AddInDB(db, "nat-gateway/ng-0/vpc", "vp-s70")
	if err := validNATGateway().Prepare(db, nil); err != nil {
		t.Errorf("Prepare a échoué : %v", err)
	}
}

func TestDeleteNATGatewayCommand_Prepare(t *testing.T) {
	for st, ok := range map[string]bool{"running": true, "error": true, "creating": false, "deleting": false} {
		_, db := newTestDispatcher(t)
		kv.AddInDB(db, "nat-gateway/ng-1/state", st)
		err := DeleteNATGatewayCommand{Name: "ng-1"}.Prepare(db, nil)
		if ok != (err == nil) {
			t.Errorf("état %s : erreur %v", st, err)
		}
		got, _ := kv.GetFromDB(db, "nat-gateway/ng-1/state")
		if ok && got != "deleting" {
			t.Errorf("état %s : %q après Prepare, deleting attendu", st, got)
		}
	}
}

func TestDeleteNATGatewayCommand_Prepare_Unknown(t *testing.T) {
	_, db := newTestDispatcher(t)
	err := DeleteNATGatewayCommand{Name: "ng-x"}.Prepare(db, nil)
	if err == nil || !strings.Contains(err.Error(), `nat gateway "ng-x" not found`) {
		t.Errorf("erreur %v", err)
	}
}

func TestDeleteVPCCommand_Prepare_RefusesWhileANATGatewayExists(t *testing.T) {
	for _, st := range []string{"running", "error", "creating", "deleting"} {
		_, db := newTestDispatcher(t)
		kv.AddInDB(db, "vpc/vp-s7/state", "running")
		kv.AddInDB(db, "nat-gateway/ng-1/state", st)
		kv.AddInDB(db, "nat-gateway/ng-1/vpc", "vp-s7")
		err := DeleteVPCCommand{Name: "vp-s7"}.Prepare(db, nil)
		if err == nil || !strings.Contains(err.Error(), `nat gateway "ng-1" must be deleted before deleting vpc "vp-s7"`) {
			t.Errorf("passerelle %s : erreur %v", st, err)
		}
		if got, _ := kv.GetFromDB(db, "vpc/vp-s7/state"); got != "running" {
			t.Errorf("passerelle %s : VPC passé en %q malgré le refus", st, got)
		}
	}
}

func TestParseNATGatewayKeepsTheHostAddress(t *testing.T) {
	ip, gw, err := parseNATGateway("198.51.100.50/24", "198.51.100.1")
	if err != nil {
		t.Fatal(err)
	}
	if ip.String() != "198.51.100.50/24" || gw.String() != "198.51.100.1" {
		t.Errorf("ip %s, gateway %s", ip, gw)
	}
}
