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
		fmt.Fprintf(&sb, "%s,%s\n", r.MAC, r.IP)
	}

	path := filepath.Join(dir, vmName)
	if err := os.WriteFile(path, []byte(sb.String()), 0644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
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
