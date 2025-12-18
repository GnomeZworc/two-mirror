package metadata

type NoCloudData struct {
	MetaData      string `json:"meta-data"`
	UserData      string `json:"user-data"`
	NetworkConfig string `json:"network-config"`
	VendorData    string `json:"vendor-data"`
}
