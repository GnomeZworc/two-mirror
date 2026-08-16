package qemu

func ScopeName(vmName string) string {
	return "two-vm-" + vmName + ".scope"
}

type DiskConfig struct {
	Path string
	Dev  string
}

type Config struct {
	Name         string
	TapID        int
	Mac          string
	Disks        []DiskConfig
	Memory       int
	CPUs         int
	UEFICodePath string
	UEFIVarsPath string
	SerialDir    string
	MonitorDir   string
	QMPDir       string
}
