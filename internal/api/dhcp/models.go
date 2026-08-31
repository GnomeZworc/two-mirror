package dhcpapi

type Verb string

const (
	VerbSetSubnet Verb = "set-subnet"
	VerbSetHost   Verb = "set-host"
	VerbDelHost   Verb = "del-host"
	VerbGetState  Verb = "get-state"
	VerbProbe     Verb = "probe"
)

const MaxMessageBytes = 64 * 1024

type Subnet struct {
	Network        string `json:"network"`
	InterfaceIP    string `json:"interface_ip"`
	VPCRoute       string `json:"vpc_route,omitempty"`
	DefaultGateway string `json:"default_gateway,omitempty"`
}

type Host struct {
	MAC          string `json:"mac"`
	IP           string `json:"ip"`
	VM           string `json:"vm,omitempty"`
	DefaultRoute bool   `json:"default_route"`
}

type State struct {
	Subnet *Subnet `json:"subnet,omitempty"`
	Hosts  []Host  `json:"hosts"`
}

type Lease struct {
	MAC          string   `json:"mac"`
	IP           string   `json:"ip"`
	Netmask      string   `json:"netmask"`
	Router       string   `json:"router,omitempty"`
	DNS          []string `json:"dns"`
	Routes       []string `json:"routes"`
	LeaseSeconds uint32   `json:"lease_seconds"`
}

type Request struct {
	Verb   Verb    `json:"verb"`
	Subnet *Subnet `json:"subnet,omitempty"`
	Host   *Host   `json:"host,omitempty"`
	MAC    string  `json:"mac,omitempty"`
}

type Response struct {
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
	State  *State `json:"state,omitempty"`
	Digest string `json:"digest,omitempty"`
	Lease  *Lease `json:"lease,omitempty"`
	Served bool   `json:"served,omitempty"`
}
