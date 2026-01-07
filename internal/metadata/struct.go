package metadata

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
	Netns    string
	File     string
	Iface    string
	Port     int
	ConfFile string
	VmName   string
}

type NoCloudConfig struct {
	VpcName  string
	BindIP   string
	BindPort string
	Name     string
	Password string
	SSHKEY   string
}
