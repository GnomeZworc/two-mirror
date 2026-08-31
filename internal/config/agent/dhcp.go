package configuration

import "fmt"

const (
	BackendDnsmasq = "dnsmasq"
	BackendTwo     = "two"
)

func ValidBackend(backend string) error {
	switch backend {
	case BackendDnsmasq, BackendTwo:
		return nil
	default:
		return fmt.Errorf("unknown dhcp backend %q: expected %q or %q", backend, BackendDnsmasq, BackendTwo)
	}
}
