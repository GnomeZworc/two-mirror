package configuration

const (
	InternetInterface   = "internet"
	DefaultPublicBridge = "br-public"
)

func (c Config) PublicBridge() string {
	if bridge := c.Interfaces[InternetInterface]; bridge != "" {
		return bridge
	}
	return DefaultPublicBridge
}
