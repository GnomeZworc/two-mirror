package agentapi

type VPCCreateRequest struct {
	Name  string  `json:"name"`
	CIDR  string  `json:"cidr"`
	L3VNI *uint32 `json:"l3vni,omitempty"`
}

type VPC struct {
	Name  string `json:"name"`
	State string `json:"state"`
	CIDR  string `json:"cidr"`
	L3VNI uint32 `json:"l3vni,omitempty"`
}

type SubnetCreateRequest struct {
	Name         string `json:"name"`
	VPC          string `json:"vpc"`
	Mode         string `json:"mode"`
	VxlanID      int    `json:"vxlan_id"`
	IfaceType    string `json:"iface_type"`
	InterfaceIP  string `json:"interface_ip"`
	CIDR         string `json:"cidr"`
	DefaultRoute bool   `json:"default_route"`
	Gateway      string `json:"gateway"`
}

type Subnet struct {
	Name         string `json:"name"`
	State        string `json:"state"`
	VPC          string `json:"vpc"`
	Mode         string `json:"mode"`
	VxlanID      int    `json:"vxlan_id,omitempty"`
	LocalIface   string `json:"local_iface"`
	InterfaceIP  string `json:"interface_ip"`
	RelayIP      string `json:"relay_ip,omitempty"`
	CIDR         string `json:"cidr"`
	DefaultRoute bool   `json:"default_route"`
	Gateway      string `json:"gateway"`
}

type VMInterface struct {
	Name       string `json:"name"`
	Subnet     string `json:"subnet"`
	IP         string `json:"ip"`
	MAC        string `json:"mac"`
	Primary    bool   `json:"primary"`
	DHCPServer bool   `json:"dhcp_server"`
}

type VMStorage struct {
	Path string `json:"path"`
	Dev  string `json:"dev"`
}

type VMMetadata struct {
	Password string `json:"password"`
	SSHKey   string `json:"sshkey"`
	UserData string `json:"user_data"`
}

type VMCreateRequest struct {
	Name       string        `json:"name"`
	Memory     int           `json:"memory"`
	CPUs       int           `json:"cpus"`
	UEFI       bool          `json:"uefi"`
	Metadata   VMMetadata    `json:"metadata"`
	Interfaces []VMInterface `json:"interfaces"`
	Storage    []VMStorage   `json:"storage"`
}

type VM struct {
	Name         string        `json:"name"`
	State        string        `json:"state"`
	MetadataPort string        `json:"metadata_port"`
	Memory       int           `json:"memory"`
	CPUs         int           `json:"cpus"`
	UEFI         bool          `json:"uefi"`
	Interfaces   []VMInterface `json:"interfaces"`
	Storage      []VMStorage   `json:"storage"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type NATGatewayCreateRequest struct {
	Name    string `json:"name"`
	VPC     string `json:"vpc"`
	IP      string `json:"ip"`
	Gateway string `json:"gateway"`
}

type NATGateway struct {
	Name    string `json:"name"`
	State   string `json:"state"`
	VPC     string `json:"vpc,omitempty"`
	IP      string `json:"ip,omitempty"`
	Gateway string `json:"gateway,omitempty"`
}
