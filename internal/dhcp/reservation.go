package dhcp

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Reservation struct {
	MAC string
	IP  string
	Tag string // pose set:<Tag> sur l'entrée, pour cibler les options par interface
}

func HostsDir(confDir, name string) string {
	return filepath.Join(confDir, name+".hosts.d")
}

func OptsDir(confDir, name string) string {
	return filepath.Join(confDir, name+".opts.d")
}

func UnitName(name string) string {
	return "dnsmasq@" + name + ".service"
}

func WriteReservations(confDir, name, vmName string, res []Reservation) error {
	if len(res) == 0 {
		return fmt.Errorf("no reservation for vm %q: it would get no address", vmName)
	}

	dir := HostsDir(confDir, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	var sb strings.Builder
	for _, r := range res {
		if r.MAC == "" || r.IP == "" {
			return fmt.Errorf("incomplete reservation for vm %q: mac=%q ip=%q", vmName, r.MAC, r.IP)
		}
		if r.Tag == "" {
			fmt.Fprintf(&sb, "%s,%s\n", r.MAC, r.IP)
			continue
		}
		fmt.Fprintf(&sb, "%s,%s,set:%s\n", r.MAC, r.IP, r.Tag)
	}

	path := filepath.Join(dir, vmName)
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// WriteVMOptions écrit les options DHCP propres à des interfaces de cette VM
// sur ce subnet : elles suppriment la route par défaut — option 3 nue — et
// réémettent les autres routes.
//
// Un override de l'option 121 remplace la précédente en entier, il ne s'y
// ajoute pas (vérifié sur dnsmasq 2.90) : omettre la route vers le serveur de
// métadonnées la ferait disparaître, et la VM ne se provisionnerait pas.
func WriteVMOptions(confDir, name, vmName string, tags []string, c Config) error {
	if len(tags) == 0 {
		return RemoveVMOptions(confDir, name, vmName)
	}
	if c.InterfaceIP == nil {
		return fmt.Errorf("interface ip is required to build options for vm %q", vmName)
	}
	if c.DefaultGateway != nil {
		return fmt.Errorf("vm options for %q must not carry a default route", vmName)
	}

	dir := OptsDir(confDir, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	routes := strings.Join(classlessRoutes(c), ",")
	var sb strings.Builder
	for _, tag := range tags {
		fmt.Fprintf(&sb, "tag:%s,3\n", tag)
		fmt.Fprintf(&sb, "tag:%s,121,%s\n", tag, routes)
	}

	path := filepath.Join(dir, vmName)
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

func RemoveVMOptions(confDir, name, vmName string) error {
	path := filepath.Join(OptsDir(confDir, name), vmName)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %s: %w", path, err)
	}
	return nil
}

func RemoveReservations(confDir, name, vmName string) error {
	for _, dir := range []string{HostsDir(confDir, name), OptsDir(confDir, name)} {
		path := filepath.Join(dir, vmName)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove %s: %w", path, err)
		}
	}
	return nil
}

func RemoveSubnetDirs(confDir, name string) error {
	for _, dir := range []string{HostsDir(confDir, name), OptsDir(confDir, name)} {
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("remove %s: %w", dir, err)
		}
	}
	return nil
}
