package render

import (
	"fmt"
	"path/filepath"
	"strconv"

	"git.g3e.fr/syonad/two/internal/lab/topology"
)

const loopback = "127.0.0.1"

func qemuArgs(p *topology.Plan, n topology.NodePlan, index int, dir string) []string {
	args := []string{
		"-name", n.Name,
		"-machine", "q35",
		"-accel", "kvm",
		"-cpu", "host",
		"-smp", strconv.Itoa(n.CPUs),
		"-m", strconv.Itoa(n.Memory),
		"-nodefaults",
		"-display", "none",
		"-serial", "file:" + filepath.Join(dir, ConsoleFile),
		"-qmp", "unix:" + filepath.Join(dir, QMPFile) + ",server=on,wait=off",
		"-pidfile", filepath.Join(dir, PIDFile),
		"-drive", "file=" + filepath.Join(dir, DiskFile) + ",if=virtio,format=qcow2",
		"-drive", "file=" + filepath.Join(dir, SeedFile) + ",media=cdrom,readonly=on",
	}

	restrict := "on"
	if n.Role == topology.RoleSwitch {
		restrict = "off"
	}
	args = append(args,
		"-netdev", fmt.Sprintf("user,id=%s,restrict=%s,ipv6=off,hostfwd=tcp:%s:%d-:22", AdminInterface, restrict, loopback, n.SSHPort),
		"-device", fmt.Sprintf("virtio-net-pci,netdev=%s,mac=%s,romfile=", AdminInterface, adminMAC(index)),
	)

	for _, c := range nodeCables(p, n.Name) {
		args = append(args, cable(c.NodeInterface, c.NodePort, c.SwitchPort, c.NodeMAC.String(), c.MTU)...)
	}
	for _, c := range switchCables(p, n.Name) {
		args = append(args, cable(c.SwitchInterface, c.SwitchPort, c.NodePort, c.SwitchMAC.String(), c.MTU)...)
	}
	return args
}

func cable(id string, local, remote int, mac string, mtu int) []string {
	return []string{
		"-netdev", fmt.Sprintf("dgram,id=%s,local.type=inet,local.host=%s,local.port=%d,remote.type=inet,remote.host=%s,remote.port=%d",
			id, loopback, local, loopback, remote),
		"-device", fmt.Sprintf("virtio-net-pci,netdev=%s,mac=%s,host_mtu=%d,romfile=", id, mac, mtu),
	}
}
