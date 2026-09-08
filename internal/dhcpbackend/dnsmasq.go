package dhcpbackend

import (
	"fmt"

	"git.g3e.fr/syonad/two/internal/dhcp"
	"git.g3e.fr/syonad/two/pkg/systemd"
)

type Dnsmasq struct {
	ConfDir string
}

func (b Dnsmasq) confDir() string {
	if b.ConfDir == "" {
		return dhcp.DefaultConfDir
	}
	return b.ConfDir
}

func tag(vmName string, index int) string {
	return fmt.Sprintf("%s-%d", vmName, index)
}

func (Dnsmasq) Unit(s Subnet) string {
	return dhcp.UnitName(s.Instance())
}

func (b Dnsmasq) config(s Subnet) dhcp.Config {
	return dhcp.Config{
		Network:        s.Network,
		Name:           s.Instance(),
		ConfDir:        b.confDir(),
		InterfaceIP:    s.InterfaceIP,
		VPCRoute:       s.VPCRoute,
		DefaultGateway: s.DefaultGateway,
	}
}

func (b Dnsmasq) ConfigureSubnet(s Subnet) error {
	if _, _, err := dhcp.GenerateConfig(b.config(s)); err != nil {
		return fmt.Errorf("generate dhcp config: %w", err)
	}

	svc, err := systemd.New()
	if err != nil {
		return fmt.Errorf("connect to systemd: %w", err)
	}
	defer svc.Close()

	if err := svc.Start(b.Unit(s)); err != nil {
		return fmt.Errorf("start dnsmasq: %w", err)
	}
	return nil
}

func (b Dnsmasq) TeardownSubnet(s Subnet) error {
	svc, err := systemd.New()
	if err != nil {
		return fmt.Errorf("connect to systemd: %w", err)
	}
	defer svc.Close()

	unit := b.Unit(s)
	if status, err := svc.Status(unit); err == nil && status.ActiveState == "active" {
		if err := svc.Stop(unit); err != nil {
			return fmt.Errorf("stop dnsmasq: %w", err)
		}
	}

	if err := dhcp.RemoveConfig(b.confDir(), s.Instance()); err != nil {
		return err
	}
	return dhcp.RemoveSubnetDirs(b.confDir(), s.Instance())
}

func (b Dnsmasq) SetVM(s Subnet, vmName string, res []Reservation) error {
	instance := s.Instance()

	reservations := make([]dhcp.Reservation, 0, len(res))
	var tags []string
	for _, r := range res {
		reservations = append(reservations, dhcp.Reservation{
			MAC: r.MAC, IP: r.IP, Tag: tag(vmName, r.Index),
		})
		if !r.DefaultRoute {
			tags = append(tags, tag(vmName, r.Index))
		}
	}

	if err := dhcp.WriteReservations(b.confDir(), instance, vmName, reservations); err != nil {
		return fmt.Errorf("write dhcp reservations on %s: %w", instance, err)
	}

	options := dhcp.Config{InterfaceIP: s.InterfaceIP, VPCRoute: s.VPCRoute}
	if err := dhcp.WriteVMOptions(b.confDir(), instance, vmName, tags, options); err != nil {
		return fmt.Errorf("write dhcp options on %s: %w", instance, err)
	}
	return nil
}

func (b Dnsmasq) DelVM(s Subnet, vmName string, _ []Reservation) error {
	if err := dhcp.RemoveReservations(b.confDir(), s.Instance(), vmName); err != nil {
		return err
	}

	svc, err := systemd.New()
	if err != nil {
		return fmt.Errorf("connect to systemd: %w", err)
	}
	defer svc.Close()

	unit := b.Unit(s)
	status, err := svc.Status(unit)
	if err != nil || status.ActiveState != "active" {
		return nil
	}
	if err := svc.Restart(unit); err != nil {
		return fmt.Errorf("restart %s: %w", unit, err)
	}
	if status, err := svc.Status(unit); err != nil {
		return fmt.Errorf("status %s after restart: %w", unit, err)
	} else if status.ActiveState != "active" {
		return fmt.Errorf("%s is %s after restart", unit, status.ActiveState)
	}
	return nil
}
