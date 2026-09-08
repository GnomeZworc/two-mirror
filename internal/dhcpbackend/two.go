package dhcpbackend

import (
	"fmt"
	"time"

	dhcpapi "git.g3e.fr/syonad/two/internal/api/dhcp"
	dhcpclient "git.g3e.fr/syonad/two/internal/client/dhcp"
	"git.g3e.fr/syonad/two/pkg/db/statefile"
	"git.g3e.fr/syonad/two/pkg/systemd"
)

const (
	readyTimeout = 5 * time.Second
	readyPoll    = 50 * time.Millisecond
)

type Two struct {
	RunDir string
}

func (b Two) runDir() string {
	if b.RunDir == "" {
		return dhcpapi.DefaultRunDir
	}
	return b.RunDir
}

func (b Two) Unit(s Subnet) string {
	return dhcpapi.Unit(s.Instance())
}

func (b Two) client(s Subnet) *dhcpclient.Client {
	return dhcpclient.New(dhcpapi.SocketPath(b.runDir(), s.Instance()))
}

func (b Two) statePath(s Subnet) string {
	return dhcpapi.StatePath(b.runDir(), s.Instance())
}

func (b Two) waitReady(s Subnet, timeout, poll time.Duration) error {
	client := b.client(s)
	deadline := time.Now().Add(timeout)

	var err error
	for {
		if _, _, err = client.GetState(); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("dhcp server for %s did not answer within %s: %w", s.Instance(), timeout, err)
		}
		time.Sleep(poll)
	}
}

func (b Two) ConfigureSubnet(s Subnet) error {
	if err := statefile.Remove(b.statePath(s)); err != nil {
		return fmt.Errorf("remove residual state: %w", err)
	}

	svc, err := systemd.New()
	if err != nil {
		return fmt.Errorf("connect to systemd: %w", err)
	}
	defer svc.Close()

	if err := svc.Start(b.Unit(s)); err != nil {
		return fmt.Errorf("start dhcp: %w", err)
	}
	if err := b.waitReady(s, readyTimeout, readyPoll); err != nil {
		return err
	}

	return b.pushSubnet(s)
}

func (b Two) pushSubnet(s Subnet) error {
	subnet := dhcpapi.Subnet{
		Network:     s.Network.String(),
		InterfaceIP: s.InterfaceIP.String(),
	}
	if s.VPCRoute != nil {
		subnet.VPCRoute = s.VPCRoute.String()
	}
	if s.DefaultGateway != nil {
		subnet.DefaultGateway = s.DefaultGateway.String()
	}

	if err := b.client(s).SetSubnet(subnet); err != nil {
		return fmt.Errorf("configure dhcp for %s: %w", s.Instance(), err)
	}
	return nil
}

func (b Two) TeardownSubnet(s Subnet) error {
	svc, err := systemd.New()
	if err != nil {
		return fmt.Errorf("connect to systemd: %w", err)
	}
	defer svc.Close()

	unit := b.Unit(s)
	if status, err := svc.Status(unit); err == nil && status.ActiveState == "active" {
		if err := svc.Stop(unit); err != nil {
			return fmt.Errorf("stop dhcp: %w", err)
		}
	}

	return statefile.Remove(b.statePath(s))
}

func (b Two) SetVM(s Subnet, vmName string, res []Reservation) error {
	client := b.client(s)

	for _, r := range res {
		host := dhcpapi.Host{
			MAC:          r.MAC,
			IP:           r.IP,
			VM:           vmName,
			DefaultRoute: r.DefaultRoute,
		}
		if err := client.SetHost(host); err != nil {
			return fmt.Errorf("reserve %s for vm %s on %s: %w", r.MAC, vmName, s.Instance(), err)
		}
	}
	return nil
}

func (b Two) DelVM(s Subnet, vmName string, res []Reservation) error {
	client := b.client(s)

	for _, r := range res {
		if err := client.DelHost(r.MAC); err != nil {
			return fmt.Errorf("release %s of vm %s on %s: %w", r.MAC, vmName, s.Instance(), err)
		}
	}
	return nil
}
