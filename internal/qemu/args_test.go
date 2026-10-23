package qemu

import (
	"testing"
)

func devicesOf(args []string) []string {
	var devices []string
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-device" {
			devices = append(devices, args[i+1])
		}
	}
	return devices
}

func TestBuildArgs_DiskSerialIsDeclaredDev(t *testing.T) {
	args, err := buildArgs(Config{
		Name: "vm1",
		NICs: []NICConfig{{TapID: 1, Mac: "52:54:00:00:00:01", MTU: 1450}, {TapID: 2, Mac: "52:54:00:00:00:02"}},
		Disks: []DiskConfig{
			{Path: "/v/b.qcow2", Dev: "xvdb"},
			{Path: "/v/root.qcow2", Dev: "vda"},
			{Path: "/v/a.qcow2", Dev: "xvda"},
		},
	})
	if err != nil {
		t.Fatalf("buildArgs a échoué : %v", err)
	}

	want := []string{
		"virtio-scsi-pci,id=scsi0,bus=pci.0,addr=0x1e",
		"virtio-blk-pci,drive=vda,bus=pci.0,addr=0x1f,bootindex=1,serial=vda",
		"scsi-hd,drive=xvda,bus=scsi0.0,scsi-id=0,bootindex=2,serial=xvda",
		"scsi-hd,drive=xvdb,bus=scsi0.0,scsi-id=1,bootindex=3,serial=xvdb",
		"virtio-net-pci,netdev=net0,mac=52:54:00:00:00:01,bus=pci.0,addr=0x03,host_mtu=1450",
		"virtio-net-pci,netdev=net1,mac=52:54:00:00:00:02,bus=pci.0,addr=0x04",
	}
	got := devicesOf(args)
	if len(got) != len(want) {
		t.Fatalf("devices = %q, attendu %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("device %d = %q, attendu %q", i, got[i], want[i])
		}
	}
}

func TestBuildArgs_EndsWithDaemonize(t *testing.T) {
	args, err := buildArgs(Config{Name: "vm1", NICs: []NICConfig{{TapID: 1, Mac: "52:54:00:00:00:01"}}})
	if err != nil {
		t.Fatalf("buildArgs a échoué : %v", err)
	}
	if args[len(args)-1] != "-daemonize" {
		t.Errorf("dernier argument = %q, attendu -daemonize", args[len(args)-1])
	}
}

func TestBuildArgs_RejectsSecondVirtioDisk(t *testing.T) {
	_, err := buildArgs(Config{
		Name:  "vm1",
		NICs:  []NICConfig{{TapID: 1, Mac: "52:54:00:00:00:01"}},
		Disks: []DiskConfig{{Path: "/v/a", Dev: "vda"}, {Path: "/v/b", Dev: "vdb"}},
	})
	if err == nil {
		t.Fatal("buildArgs a accepté deux disques virtio")
	}
}

func TestBuildArgs_RejectsNoNIC(t *testing.T) {
	_, err := buildArgs(Config{Name: "vm1"})
	if err == nil {
		t.Fatal("buildArgs a accepté une VM sans interface")
	}
}
