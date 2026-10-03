package render

import (
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"

	"git.g3e.fr/syonad/two/internal/lab/topology"
)

const (
	SwitchScript = "/usr/local/sbin/lab-switch"
	SwitchNFT    = "/etc/lab-switch.nft"
	SwitchUnit   = "/etc/systemd/system/lab-switch.service"
)

type metaDoc struct {
	InstanceID    string `yaml:"instance-id"`
	LocalHostname string `yaml:"local-hostname"`
}

type writeFile struct {
	Path        string `yaml:"path"`
	Permissions string `yaml:"permissions"`
	Content     string `yaml:"content"`
}

type cloudConfig struct {
	Hostname          string      `yaml:"hostname"`
	SSHPwauth         bool        `yaml:"ssh_pwauth"`
	DisableRoot       bool        `yaml:"disable_root"`
	SSHAuthorizedKeys []string    `yaml:"ssh_authorized_keys"`
	Packages          []string    `yaml:"packages,omitempty"`
	WriteFiles        []writeFile `yaml:"write_files,omitempty"`
	Runcmd            [][]string  `yaml:"runcmd,omitempty"`
}

type match struct {
	MACAddress string `yaml:"macaddress"`
}

type route struct {
	To  string `yaml:"to"`
	Via string `yaml:"via"`
}

type nameservers struct {
	Addresses []string `yaml:"addresses"`
}

type ethernet struct {
	Match       match        `yaml:"match"`
	SetName     string       `yaml:"set-name"`
	DHCP4       bool         `yaml:"dhcp4"`
	MTU         int          `yaml:"mtu,omitempty"`
	Addresses   []string     `yaml:"addresses,omitempty"`
	Routes      []route      `yaml:"routes,omitempty"`
	Nameservers *nameservers `yaml:"nameservers,omitempty"`
}

type networkDoc struct {
	Version   int                 `yaml:"version"`
	Ethernets map[string]ethernet `yaml:"ethernets"`
}

func metaData(p *topology.Plan, n topology.NodePlan) ([]byte, error) {
	return yaml.Marshal(metaDoc{InstanceID: p.Name + "-" + n.Name, LocalHostname: n.Name})
}

func userData(p *topology.Plan, n topology.NodePlan, keys []string) ([]byte, error) {
	cfg := cloudConfig{
		Hostname:          n.Name,
		SSHPwauth:         false,
		DisableRoot:       true,
		SSHAuthorizedKeys: keys,
	}
	if n.Role == topology.RoleSwitch {
		cfg.Packages = []string{"nftables"}
		cfg.WriteFiles = []writeFile{
			{Path: SwitchScript, Permissions: "0755", Content: switchScript(p, n.Name)},
			{Path: SwitchNFT, Permissions: "0644", Content: switchNFT(p, n.Name)},
			{Path: SwitchUnit, Permissions: "0644", Content: switchUnit()},
		}
		cfg.Runcmd = [][]string{
			{"systemctl", "daemon-reload"},
			{"systemctl", "enable", "--now", "lab-switch.service"},
		}
	}
	body, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	return append([]byte("#cloud-config\n"), body...), nil
}

func networkConfig(p *topology.Plan, n topology.NodePlan, index int) ([]byte, error) {
	doc := networkDoc{Version: 2, Ethernets: map[string]ethernet{}}
	admin := ethernet{
		Match:     match{MACAddress: adminMAC(index).String()},
		SetName:   AdminInterface,
		Addresses: []string{AdminAddress},
	}
	if n.Role == topology.RoleSwitch {
		admin.Routes = []route{{To: "0.0.0.0/0", Via: AdminGateway}}
		admin.Nameservers = &nameservers{Addresses: []string{AdminDNS}}
		for _, c := range switchCables(p, n.Name) {
			doc.Ethernets[c.SwitchInterface] = ethernet{
				Match:   match{MACAddress: c.SwitchMAC.String()},
				SetName: c.SwitchInterface,
				MTU:     c.MTU,
			}
		}
	}
	doc.Ethernets[AdminInterface] = admin

	for i, c := range nodeCables(p, n.Name) {
		e := ethernet{
			Match:     match{MACAddress: c.NodeMAC.String()},
			SetName:   c.NodeInterface,
			MTU:       c.MTU,
			Addresses: []string{c.NodeAddress.String()},
		}
		if i == 0 {
			e.Routes = []route{{To: "0.0.0.0/0", Via: gatewayOf(p, c.Segment)}}
			e.Nameservers = &nameservers{Addresses: Nameservers}
		}
		doc.Ethernets[c.NodeInterface] = e
	}
	return yaml.Marshal(doc)
}

func gatewayOf(p *topology.Plan, segment string) string {
	for _, s := range p.Segments {
		if s.Name == segment {
			return s.Gateway.String()
		}
	}
	return ""
}

func switchScript(p *topology.Plan, name string) string {
	var b strings.Builder
	b.WriteString("#!/bin/sh\nset -eu\nsysctl -qw net.ipv4.ip_forward=1\n")
	for _, s := range switchSegments(p, name) {
		fmt.Fprintf(&b, "ip link add %s type bridge stp_state 0 2>/dev/null || true\n", s.Bridge)
		for _, c := range switchCables(p, name) {
			if c.Segment != s.Name {
				continue
			}
			fmt.Fprintf(&b, "ip link set dev %s master %s\n", c.SwitchInterface, s.Bridge)
			fmt.Fprintf(&b, "ip link set dev %s up\n", c.SwitchInterface)
		}
		fmt.Fprintf(&b, "ip link set dev %s mtu %d\n", s.Bridge, s.MTU)
		fmt.Fprintf(&b, "ip addr replace %s/%d dev %s\n", s.Gateway, s.Network.Bits(), s.Bridge)
		fmt.Fprintf(&b, "ip link set dev %s up\n", s.Bridge)
	}
	fmt.Fprintf(&b, "nft -f %s\n", SwitchNFT)
	return b.String()
}

func switchNFT(p *topology.Plan, name string) string {
	var networks []string
	for _, s := range switchSegments(p, name) {
		networks = append(networks, s.Network.String())
	}
	return fmt.Sprintf(`add table ip lab_nat
delete table ip lab_nat
table ip lab_nat {
	chain postrouting {
		type nat hook postrouting priority srcnat; policy accept;
		ip saddr { %s } oifname "%s" masquerade
	}
}
`, strings.Join(networks, ", "), AdminInterface)
}

func switchUnit() string {
	return fmt.Sprintf(`[Unit]
Description=Lab switch: bridges, gateways and NAT
Wants=network-online.target
After=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=%s

[Install]
WantedBy=multi-user.target
`, SwitchScript)
}
