package subnet

const (
	ModeVxlan    = "vxlan"
	ModeBridge   = "bridge"
	ModePublicIP = "public_ip"
)

func ValidMode(mode string) bool {
	switch mode {
	case ModeVxlan, ModeBridge, ModePublicIP:
		return true
	}
	return false
}

func Bridged(mode string) bool {
	return mode == ModeBridge || mode == ModePublicIP
}
