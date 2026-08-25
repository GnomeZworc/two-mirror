package qemu

func ScopeName(vmName string) string {
	return "two-vm-" + vmName + ".scope"
}

type DiskConfig struct {
	Path string
	Dev  string
}

// NICConfig décrit une interface réseau. Sa position dans Config.NICs
// détermine le slot PCI, donc le nom de l'interface dans le guest.
type NICConfig struct {
	TapID int
	Mac   string
}

type Config struct {
	Name         string
	NICs         []NICConfig
	Disks        []DiskConfig
	Memory       int
	CPUs         int
	UEFICodePath string
	UEFIVarsPath string
	SerialDir    string
	MonitorDir   string
	QMPDir       string
}
