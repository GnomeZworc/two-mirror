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

type Config struct {
	Netns    string
	File     string
	Iface    string
	Port     int
	ConfFile string
	VmName   string
}
