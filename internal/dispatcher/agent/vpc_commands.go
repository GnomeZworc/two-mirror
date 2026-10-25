package dispatcher

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/l3vni"
	"git.g3e.fr/syonad/two/internal/netif"
	"git.g3e.fr/syonad/two/internal/state"
	"git.g3e.fr/syonad/two/internal/vpc"
	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
)

type CreateVPCCommand struct {
	Name  string
	CIDR  string
	L3VNI uint32
}

func (c CreateVPCCommand) Key() string { return "vpc/" + c.Name }

func (c CreateVPCCommand) Prepare(db *badger.DB, cfg *configuration.Config) error {
	if _, err := kv.GetFromDB(db, "vpc/"+c.Name+"/state"); err == nil {
		return fmt.Errorf("vpc %q already exists", c.Name)
	}
	if _, _, err := net.ParseCIDR(c.CIDR); err != nil {
		return fmt.Errorf("invalid cidr %q: %w", c.CIDR, err)
	}
	if c.L3VNI != 0 {
		if err := c.checkL3VNI(db, cfg); err != nil {
			return err
		}
		if err := checkUnderlay(cfg, cfg.DefaultInterface); err != nil {
			return err
		}
	}
	if err := kv.AddInDB(db, "vpc/"+c.Name+"/cidr", c.CIDR); err != nil {
		return err
	}
	if c.L3VNI != 0 {
		if err := kv.AddInDB(db, "vpc/"+c.Name+"/l3vni", strconv.FormatUint(uint64(c.L3VNI), 10)); err != nil {
			return err
		}
	}
	return state.Set(db, c.Key(), state.Creating)
}

var LinkMTU = netif.LinkMTU

func checkUnderlay(cfg *configuration.Config, iface string) error {
	m, err := LinkMTU(iface)
	if err != nil {
		return fmt.Errorf("read mtu of underlay %s: %w", iface, err)
	}
	return cfg.Network.CheckUnderlay(iface, m)
}

func checkBridge(cfg *configuration.Config, iface string) error {
	m, err := LinkMTU(iface)
	if err != nil {
		return fmt.Errorf("read mtu of bridge %s: %w", iface, err)
	}
	return cfg.Network.CheckBridge(iface, m)
}

func (c CreateVPCCommand) checkL3VNI(db *badger.DB, cfg *configuration.Config) error {
	if err := l3vni.Valid(c.L3VNI); err != nil {
		return err
	}
	if cfg == nil || !cfg.BGP.Enabled() {
		return fmt.Errorf("l3vni requires the bgp section in the agent configuration")
	}
	entries, err := kv.ListByPrefix(db, "vpc/")
	if err != nil {
		return fmt.Errorf("failed to list vpcs: %w", err)
	}
	want := strconv.FormatUint(uint64(c.L3VNI), 10)
	for key, value := range entries {
		parts := strings.Split(key, "/")
		if len(parts) != 3 || parts[2] != "l3vni" || value != want {
			continue
		}
		if s, err := state.Get(db, "vpc/"+parts[1]); err == nil && s == state.Deleted {
			continue
		}
		return fmt.Errorf("l3vni %d is already used by vpc %q", c.L3VNI, parts[1])
	}
	return nil
}

func (c CreateVPCCommand) Execute(db *badger.DB, cfg *configuration.Config) error {
	return vpc.CreateVPC(db, c.Name, cfg)
}

type DeleteVPCCommand struct {
	Name string
}

func (c DeleteVPCCommand) Key() string { return "vpc/" + c.Name }

func (c DeleteVPCCommand) Prepare(db *badger.DB, _ *configuration.Config) error {
	current, err := state.Get(db, c.Key())
	if err != nil {
		return fmt.Errorf("vpc %q not found", c.Name)
	}
	if !state.CanDelete(current) {
		return fmt.Errorf("vpc %q cannot be deleted while %s", c.Name, current)
	}
	entries, err := kv.ListByPrefix(db, "subnet/")
	if err != nil {
		return fmt.Errorf("failed to list subnets: %w", err)
	}
	for key, value := range entries {
		if !strings.HasSuffix(key, "/vpc") || value != c.Name {
			continue
		}
		subnetName := strings.Split(key, "/")[1]
		s, err := state.Get(db, "subnet/"+subnetName)
		if err != nil || (s != state.Deleting && s != state.Deleted) {
			return fmt.Errorf("subnet %q must be deleted before deleting vpc %q", subnetName, c.Name)
		}
	}
	return state.Set(db, c.Key(), state.Deleting)
}

func (c DeleteVPCCommand) Execute(db *badger.DB, cfg *configuration.Config) error {
	if err := waitSubnetsDeleted(db, c.Name, cfg.Times.VPCStop(), cfg.Times.Poll()); err != nil {
		return err
	}
	if err := vpc.DeleteVPC(db, c.Name); err != nil {
		return err
	}
	current, err := state.Get(db, c.Key())
	if err != nil {
		return err
	}
	if current == state.Deleted {
		kv.DeleteInDB(db, "vpc/"+c.Name)
	}
	return nil
}

func waitSubnetsDeleted(db *badger.DB, vpcName string, timeout, poll time.Duration) error {
	err := waitUntil(timeout, poll, func() (bool, error) {
		entries, err := kv.ListByPrefix(db, "subnet/")
		if err != nil {
			return false, fmt.Errorf("failed to list subnets: %w", err)
		}
		pending := false
		for key, value := range entries {
			if !strings.HasSuffix(key, "/vpc") || value != vpcName {
				continue
			}
			subnetName := strings.Split(key, "/")[1]
			s, err := state.Get(db, "subnet/"+subnetName)
			if err != nil {
				continue
			}
			switch s {
			case state.Deleted:
			case state.Deleting:
				pending = true
			default:
				return false, fmt.Errorf("subnet %q is %s, vpc %q cannot be deleted", subnetName, s, vpcName)
			}
		}
		return !pending, nil
	})
	if errors.Is(err, errWaitTimeout) {
		return fmt.Errorf("timed out waiting for subnets of vpc %q to be deleted", vpcName)
	}
	return err
}
