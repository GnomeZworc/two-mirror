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
	LocalIP   string `json:"local_ip"`
	GatewayIP string `json:"gateway_ip"`
	CIDR      string `json:"cidr"`
}

type Subnet struct {
	Name      string `json:"name"`
	State     string `json:"state"`
	VPC       string `json:"vpc"`
	VxlanID   int    `json:"vxlan_id"`
	LocalIP   string `json:"local_ip"`
	GatewayIP string `json:"gateway_ip"`
	CIDR      string `json:"cidr"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}
