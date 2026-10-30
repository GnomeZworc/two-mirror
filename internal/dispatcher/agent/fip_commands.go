package dispatcher

import (
	"fmt"
	"net"
	"strings"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/internal/fip"
	"git.g3e.fr/syonad/two/internal/state"
	"git.g3e.fr/syonad/two/internal/subnet"
	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
)

const floatingIPPrefix = "floating-ip/"

type CreateFloatingIPCommand struct {
	Name      string
	VM        string
	Interface string
	IP        string
	Gateway   string
}

func (c CreateFloatingIPCommand) Key() string { return floatingIPPrefix + c.Name }

func (c CreateFloatingIPCommand) Prepare(db *badger.DB, _ *configuration.Config) error {
	if _, err := kv.GetFromDB(db, c.Key()+"/state"); err == nil {
		return fmt.Errorf("floating ip %q already exists", c.Name)
	}
	public, _, err := parseNATGateway(c.IP, c.Gateway)
	if err != nil {
		return err
	}
	vmState, err := state.Get(db, "vm/"+c.VM)
	if err != nil {
		return fmt.Errorf("vm %q not found", c.VM)
	}
	if vmState != state.Running {
		return fmt.Errorf("vm %q is %s", c.VM, vmState)
	}
	target, err := fip.Resolve(db, c.VM, c.Interface)
	if err != nil {
		return err
	}
	if target.Mode == subnet.ModePublicIP {
		return fmt.Errorf("interface %q of vm %q is on public_ip subnet %q: it carries its public ip itself", c.Interface, c.VM, target.Subnet)
	}
	others, err := floatingIPs(db)
	if err != nil {
		return err
	}
	for name, o := range others {
		if o.vm == c.VM && o.iface == c.Interface {
			return fmt.Errorf("interface %q of vm %q already has floating ip %q", c.Interface, c.VM, name)
		}
		if o.public != nil && o.public.Equal(public.IP) {
			return fmt.Errorf("public ip %s is already used by floating ip %q", public.IP, name)
		}
	}
	for key, value := range map[string]string{"vm": c.VM, "interface": c.Interface, "ip": c.IP, "gateway": c.Gateway} {
		if err := kv.AddInDB(db, c.Key()+"/"+key, value); err != nil {
			return err
		}
	}
	return state.Set(db, c.Key(), state.Creating)
}

func (c CreateFloatingIPCommand) Execute(db *badger.DB, _ *configuration.Config) error {
	public, gw, err := parseNATGateway(c.IP, c.Gateway)
	if err != nil {
		return err
	}
	target, err := fip.Resolve(db, c.VM, c.Interface)
	if err != nil {
		return err
	}
	if err := fip.Create(target, public, gw); err != nil {
		return err
	}
	return state.Set(db, c.Key(), state.Running)
}

type DeleteFloatingIPCommand struct {
	Name string
}

func (c DeleteFloatingIPCommand) Key() string { return floatingIPPrefix + c.Name }

func (c DeleteFloatingIPCommand) Prepare(db *badger.DB, _ *configuration.Config) error {
	current, err := state.Get(db, c.Key())
	if err != nil {
		return fmt.Errorf("floating ip %q not found", c.Name)
	}
	if !state.CanDelete(current) {
		return fmt.Errorf("floating ip %q cannot be deleted while %s", c.Name, current)
	}
	return state.Set(db, c.Key(), state.Deleting)
}

func (c DeleteFloatingIPCommand) Execute(db *badger.DB, _ *configuration.Config) error {
	entries, err := kv.ListByPrefix(db, c.Key()+"/")
	if err != nil {
		return fmt.Errorf("floating ip %q: %w", c.Name, err)
	}
	prefix := c.Key() + "/"
	public, _, err := parseNATGateway(entries[prefix+"ip"], entries[prefix+"gateway"])
	if err != nil {
		return fmt.Errorf("floating ip %q: %w", c.Name, err)
	}
	target, err := fip.Resolve(db, entries[prefix+"vm"], entries[prefix+"interface"])
	if err != nil {
		return err
	}
	if err := fip.Delete(target, public); err != nil {
		return err
	}
	if err := state.Set(db, c.Key(), state.Deleted); err != nil {
		return err
	}
	return kv.DeleteInDB(db, c.Key())
}

type floatingIPRef struct {
	vm     string
	iface  string
	public net.IP
}

func floatingIPs(db *badger.DB) (map[string]floatingIPRef, error) {
	entries, err := kv.ListByPrefix(db, floatingIPPrefix)
	if err != nil {
		return nil, fmt.Errorf("failed to list floating ips: %w", err)
	}
	refs := map[string]floatingIPRef{}
	for key, value := range entries {
		parts := strings.Split(strings.TrimPrefix(key, floatingIPPrefix), "/")
		if len(parts) != 2 {
			continue
		}
		r := refs[parts[0]]
		switch parts[1] {
		case "vm":
			r.vm = value
		case "interface":
			r.iface = value
		case "ip":
			if ip, _, err := net.ParseCIDR(value); err == nil {
				r.public = ip
			}
		default:
			continue
		}
		refs[parts[0]] = r
	}
	return refs, nil
}

func floatingIPOf(db *badger.DB, vmName string) (string, error) {
	refs, err := floatingIPs(db)
	if err != nil {
		return "", err
	}
	for name, r := range refs {
		if r.vm == vmName {
			return name, nil
		}
	}
	return "", nil
}
