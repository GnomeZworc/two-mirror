package metadata

// ServiceIP is the well-known address guests use to reach the metadata server.
// Traffic to it is DNATed to the subnet gateway by internal/iptables, and the
// route to it is advertised by internal/dhcp.
const ServiceIP = "169.254.169.254"

type NoCloudData struct {
	MetaData      string
	UserData      string
	NetworkConfig string
	VendorData    string
	NetNs         string
	Iface         string
	Port          int
}

type ServerConfig struct {
	VmName string
	RunDir string
}

type NoCloudConfig struct {
	VpcName   string
	BindIP    string
	BindPort  string
	Name      string
	Password  string
	SSHKEY    string
	Documents map[string]string
}
