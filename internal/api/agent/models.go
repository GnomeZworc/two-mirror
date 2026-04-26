package agentapi

type VPCCreateRequest struct {
	Name string `json:"name"`
}

type VPC struct {
	Name  string `json:"name"`
	State string `json:"state"`
}

type SubnetCreateRequest struct {
	Name      string `json:"name"`
	VPC       string `json:"vpc"`
	VxlanID   int    `json:"vxlan_id"`
	IfaceType string `json:"iface_type"`
	GatewayIP string `json:"gateway_ip"`
	CIDR      string `json:"cidr"`
}

type Subnet struct {
	Name       string `json:"name"`
	State      string `json:"state"`
	VPC        string `json:"vpc"`
	VxlanID    int    `json:"vxlan_id"`
	LocalIface string `json:"local_iface"`
	GatewayIP  string `json:"gateway_ip"`
	CIDR       string `json:"cidr"`
}

type VMInterface struct {
	Subnet  string `json:"subnet"`
	IP      string `json:"ip"`
	Primary bool   `json:"primary"`
}

type VMStorage struct {
	Path string `json:"path"`
	Dev  string `json:"dev"`
}

type VMCreateRequest struct {
	Name         string        `json:"name"`
	MetadataPort string        `json:"metadata_port"`
	Memory       int           `json:"memory"`
	CPUs         int           `json:"cpus"`
	Password     string        `json:"password"`
	SSHKey       string        `json:"sshkey"`
	Interfaces   []VMInterface `json:"interfaces"`
	Storage      []VMStorage   `json:"storage"`
}

type VM struct {
	Name         string        `json:"name"`
	State        string        `json:"state"`
	MetadataPort string        `json:"metadata_port"`
	Memory       int           `json:"memory"`
	CPUs         int           `json:"cpus"`
	Interfaces   []VMInterface `json:"interfaces"`
	Storage      []VMStorage   `json:"storage"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}
