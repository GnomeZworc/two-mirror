package configuration

import (
	"fmt"
	"net"
	"strconv"
)

const (
	RateLimitPerVPC = 100
	DNSPort         = 53
	NTPPort         = 123
)

type HostSvc struct {
	DNSUpstream     []string `mapstructure:"dns_upstream"`
	NTPUpstream     []string `mapstructure:"ntp_upstream"`
	RateLimitPerVPC int      `mapstructure:"rate_limit_per_vpc"`
}

func (h HostSvc) DNS() []string { return withPort(h.DNSUpstream, DNSPort) }
func (h HostSvc) NTP() []string { return withPort(h.NTPUpstream, NTPPort) }

func (h HostSvc) Rate() int {
	if h.RateLimitPerVPC == 0 {
		return RateLimitPerVPC
	}
	return h.RateLimitPerVPC
}

func withPort(servers []string, port int) []string {
	out := make([]string, 0, len(servers))
	for _, s := range servers {
		if net.ParseIP(s) != nil {
			out = append(out, net.JoinHostPort(s, strconv.Itoa(port)))
			continue
		}
		out = append(out, s)
	}
	return out
}

func (h HostSvc) Validate() error {
	if h.RateLimitPerVPC <= 0 {
		return fmt.Errorf("hostsvc.rate_limit_per_vpc %d must be a positive number of requests per second", h.RateLimitPerVPC)
	}
	for _, list := range []struct {
		key     string
		servers []string
	}{
		{"dns_upstream", h.DNSUpstream},
		{"ntp_upstream", h.NTPUpstream},
	} {
		for _, s := range list.servers {
			if err := validServer(s); err != nil {
				return fmt.Errorf("hostsvc.%s: %w", list.key, err)
			}
		}
	}
	return nil
}

func validServer(s string) error {
	if net.ParseIP(s) != nil {
		return nil
	}
	host, port, err := net.SplitHostPort(s)
	if err != nil {
		return fmt.Errorf("%q is neither an ip nor ip:port", s)
	}
	if net.ParseIP(host) == nil {
		return fmt.Errorf("%q: %q is not an ip address", s, host)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("%q: invalid port %q", s, port)
	}
	return nil
}
