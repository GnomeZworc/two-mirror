package dispatcher

import (
	"fmt"
	"net"
	"strings"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/natgw"
	"git.g3e.fr/syonad/two/internal/state"
	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
)

const natGatewayPrefix = "nat-gateway/"

type CreateNATGatewayCommand struct {
	Name    string
	VPC     string
	IP      string
	Gateway string
}

func (c CreateNATGatewayCommand) Key() string { return natGatewayPrefix + c.Name }

func (c CreateNATGatewayCommand) Prepare(db *badger.DB, _ *configuration.Config) error {
	if _, err := kv.GetFromDB(db, c.Key()+"/state"); err == nil {
		return fmt.Errorf("nat gateway %q already exists", c.Name)
	}
	if _, _, err := parseNATGateway(c.IP, c.Gateway); err != nil {
		return err
	}
	vpcState, err := state.Get(db, "vpc/"+c.VPC)
	if err != nil {
		return fmt.Errorf("vpc %q not found", c.VPC)
	}
	if vpcState != state.Running {
		return fmt.Errorf("vpc %q is %s", c.VPC, vpcState)
	}
	if _, err := kv.GetFromDB(db, "vpc/"+c.VPC+"/l3vni"); err != nil {
		return fmt.Errorf("vpc %q has no l3vni", c.VPC)
	}
	other, err := natGatewayOf(db, c.VPC)
	if err != nil {
		return err
	}
	if other != "" {
		return fmt.Errorf("vpc %q already has nat gateway %q", c.VPC, other)
	}
	for key, value := range map[string]string{"vpc": c.VPC, "ip": c.IP, "gateway": c.Gateway} {
		if err := kv.AddInDB(db, c.Key()+"/"+key, value); err != nil {
			return err
		}
	}
	return state.Set(db, c.Key(), state.Creating)
}

func (c CreateNATGatewayCommand) Execute(db *badger.DB, _ *configuration.Config) error {
	ip, gw, err := parseNATGateway(c.IP, c.Gateway)
	if err != nil {
		return err
	}
	if err := natgw.Create(db, c.VPC, ip, gw); err != nil {
		return err
	}
	return state.Set(db, c.Key(), state.Running)
}

type DeleteNATGatewayCommand struct {
	Name string
}

func (c DeleteNATGatewayCommand) Key() string { return natGatewayPrefix + c.Name }

func (c DeleteNATGatewayCommand) Prepare(db *badger.DB, _ *configuration.Config) error {
	current, err := state.Get(db, c.Key())
	if err != nil {
		return fmt.Errorf("nat gateway %q not found", c.Name)
	}
	if !state.CanDelete(current) {
		return fmt.Errorf("nat gateway %q cannot be deleted while %s", c.Name, current)
	}
	return state.Set(db, c.Key(), state.Deleting)
}

func (c DeleteNATGatewayCommand) Execute(db *badger.DB, _ *configuration.Config) error {
	vpcName, err := kv.GetFromDB(db, c.Key()+"/vpc")
	if err != nil {
		return fmt.Errorf("nat gateway %q: reading vpc: %w", c.Name, err)
	}
	if err := natgw.Delete(db, vpcName); err != nil {
		return err
	}
	if err := state.Set(db, c.Key(), state.Deleted); err != nil {
		return err
	}
	return kv.DeleteInDB(db, c.Key())
}

func parseNATGateway(ip, gateway string) (*net.IPNet, net.IP, error) {
	addr, prefix, err := net.ParseCIDR(ip)
	if err != nil || addr.To4() == nil {
		return nil, nil, fmt.Errorf("invalid ip %q: an IPv4 address with its prefix length is expected", ip)
	}
	prefix.IP = addr.To4()
	gw := net.ParseIP(gateway).To4()
	if gw == nil {
		return nil, nil, fmt.Errorf("invalid gateway %q: an IPv4 address is expected", gateway)
	}
	return prefix, gw, nil
}

func natGatewayOf(db *badger.DB, vpcName string) (string, error) {
	entries, err := kv.ListByPrefix(db, natGatewayPrefix)
	if err != nil {
		return "", fmt.Errorf("failed to list nat gateways: %w", err)
	}
	for key, value := range entries {
		name, ok := strings.CutSuffix(strings.TrimPrefix(key, natGatewayPrefix), "/vpc")
		if ok && !strings.Contains(name, "/") && value == vpcName {
			return name, nil
		}
	}
	return "", nil
}
