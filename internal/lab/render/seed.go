package render

import (
	_ "embed"
	"encoding/base64"
	"fmt"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"

	"git.g3e.fr/syonad/two/internal/lab/topology"
)

const (
	SwitchScript = "/usr/local/sbin/lab-switch"
	SwitchNFT    = "/etc/lab-switch.nft"
	SwitchUnit   = "/etc/systemd/system/lab-switch.service"

	NodeScript = "/usr/local/sbin/lab-node"
	NodeUnit   = "/etc/systemd/system/lab-node.service"

	FRRKey      = "/usr/share/keyrings/frrouting.gpg"
	FRRConfig   = "/etc/lab/frr.conf"
	FRRScript   = "/usr/local/sbin/lab-frr"
	FRRSuite    = "frr-stable"
	FRRRepo     = "https://deb.frrouting.org/frr"
	FRRPackages = "frr frr-pythontools"

	ProvisionScript   = "/usr/local/sbin/lab-provision"
	NodeProvision     = "/usr/local/sbin/lab-node-provision"
	NodePostProvision = "/usr/local/sbin/lab-node-post-provision"
	FilesDir          = "/etc/lab/files"
)

//go:embed frrouting.gpg
var frrKey []byte

type metaDoc struct {
	InstanceID    string `yaml:"instance-id"`
	LocalHostname string `yaml:"local-hostname"`
}

type writeFile struct {
	Path        string `yaml:"path"`
	Permissions string `yaml:"permissions"`
	Encoding    string `yaml:"encoding,omitempty"`
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

func userData(p *topology.Plan, n topology.NodePlan, o Options) ([]byte, error) {
	cfg := cloudConfig{
		Hostname:          n.Name,
		SSHPwauth:         false,
		DisableRoot:       true,
		SSHAuthorizedKeys: o.AuthorizedKeys,
	}
	var steps []string
	switch {
	case n.Role == topology.RoleSwitch:
		cfg.Packages = []string{"nftables"}
		cfg.WriteFiles = []writeFile{
			{Path: SwitchScript, Permissions: "0755", Content: switchScript(p, n)},
			{Path: SwitchNFT, Permissions: "0644", Content: switchNFT(p, n.Name)},
			{Path: SwitchUnit, Permissions: "0644", Content: unit("Lab switch: bridges, gateways and NAT", SwitchScript)},
		}
		steps = append(steps, "systemctl daemon-reload", "systemctl enable --now lab-switch.service")
	case n.Loopback.IsValid():
		cfg.WriteFiles = []writeFile{
			{Path: NodeScript, Permissions: "0755", Content: "#!/bin/sh\nset -eu\n" + loopbackLines(n)},
			{Path: NodeUnit, Permissions: "0644", Content: unit("Lab node: loopback", NodeScript)},
		}
		steps = append(steps, "systemctl daemon-reload", "systemctl enable --now lab-node.service")
	}
	names := make([]string, 0, len(n.Files))
	for name := range n.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		cfg.WriteFiles = append(cfg.WriteFiles, writeFile{Path: FilesDir + "/" + name, Permissions: "0644", Encoding: "b64", Content: base64.StdEncoding.EncodeToString(o.Files[n.Name][name])})
	}
	if n.Provision != "" {
		cfg.WriteFiles = append(cfg.WriteFiles, writeFile{Path: NodeProvision, Permissions: "0755", Encoding: "b64", Content: base64.StdEncoding.EncodeToString([]byte(o.Provision[n.Name]))})
		steps = append(steps, provisionEnv(p, n)+NodeProvision)
	}
	if n.FRR != "" {
		cfg.WriteFiles = append(cfg.WriteFiles,
			writeFile{Path: FRRKey, Permissions: "0644", Encoding: "b64", Content: base64.StdEncoding.EncodeToString(frrKey)},
			writeFile{Path: FRRConfig, Permissions: "0640", Content: o.FRR[n.Name]},
			writeFile{Path: FRRScript, Permissions: "0755", Content: frrScript(n)},
		)
		steps = append(steps, FRRScript)
	}
	if n.PostProvision != "" {
		cfg.WriteFiles = append(cfg.WriteFiles, writeFile{Path: NodePostProvision, Permissions: "0755", Encoding: "b64", Content: base64.StdEncoding.EncodeToString([]byte(o.PostProvision[n.Name]))})
		steps = append(steps, provisionEnv(p, n)+NodePostProvision)
	}
	if len(steps) > 0 {
		cfg.WriteFiles = append(cfg.WriteFiles,
			writeFile{Path: ProvisionScript, Permissions: "0755", Content: "#!/bin/sh\nset -eu\n" + strings.Join(steps, "\n") + "\n"},
		)
		cfg.Runcmd = [][]string{{ProvisionScript}}
	}
	body, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	return append([]byte("#cloud-config\n"), body...), nil
}

func retry(command string) string {
	return "n=0\nuntil " + command + "; do\n    n=$((n + 1))\n    [ \"$n\" -lt 30 ] || exit 1\n    sleep 10\ndone\n"
}

func provisionEnv(p *topology.Plan, n topology.NodePlan) string {
	uplink := ""
	if cables := nodeCables(p, n.Name); len(cables) > 0 {
		uplink = cables[0].NodeInterface
	}
	return fmt.Sprintf("LAB_NODE=%s LAB_UPLINK=%s LAB_FILES=%s ", n.Name, uplink, FilesDir)
}

func frrDaemons(n topology.NodePlan) []string {
	if n.Role == topology.RoleHost {
		return []string{"bgpd"}
	}
	return []string{"bgpd", "bfdd"}
}

func frrScript(n topology.NodePlan) string {
	var b strings.Builder
	b.WriteString("#!/bin/sh\nset -eu\nexport DEBIAN_FRONTEND=noninteractive\n. /etc/os-release\n")
	fmt.Fprintf(&b, "echo \"deb [signed-by=%s] %s ${VERSION_CODENAME} %s\" > /etc/apt/sources.list.d/frr.list\n", FRRKey, FRRRepo, FRRSuite)
	b.WriteString(retry("apt-get update -qq --error-on=any"))
	fmt.Fprintf(&b, "apt-get install -y -qq --no-install-recommends %s\n", FRRPackages)
	for _, d := range frrDaemons(n) {
		fmt.Fprintf(&b, "sed -i 's/^%s=no/%s=yes/' /etc/frr/daemons\n", d, d)
	}
	fmt.Fprintf(&b, "install -o frr -g frr -m 0640 %s /etc/frr/frr.conf\n", FRRConfig)
	b.WriteString("systemctl restart frr\n")
	return b.String()
}

func loopbackLines(n topology.NodePlan) string {
	if !n.Loopback.IsValid() {
		return ""
	}
	return fmt.Sprintf("ip link add %s type dummy 2>/dev/null || true\nip addr replace %s dev %s\nip link set dev %s up\n",
		topology.LoopbackInterface, n.Loopback, topology.LoopbackInterface, topology.LoopbackInterface)
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
		for _, prefix := range n.Secondary[c.Segment] {
			e.Addresses = append(e.Addresses, prefix.String())
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

func switchScript(p *topology.Plan, n topology.NodePlan) string {
	name := n.Name
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
		for _, prefix := range n.Secondary[s.Name] {
			fmt.Fprintf(&b, "ip addr replace %s dev %s\n", prefix, s.Bridge)
		}
		fmt.Fprintf(&b, "ip link set dev %s up\n", s.Bridge)
	}
	b.WriteString(loopbackLines(n))
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

func unit(description, script string) string {
	return fmt.Sprintf(`[Unit]
Description=%s
Wants=network-online.target
After=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=%s

[Install]
WantedBy=multi-user.target
`, description, script)
}
