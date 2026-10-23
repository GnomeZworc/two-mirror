package qemu

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

const (
	firstNICSlot = 0x03
	lastNICSlot  = 0x1d
	maxNICs      = lastNICSlot - firstNICSlot + 1
)

func buildArgs(cfg Config) ([]string, error) {
	memory := cfg.Memory
	if memory == 0 {
		memory = 512
	}

	cpus := cfg.CPUs
	if cpus == 0 {
		cpus = 1
	}

	serialSock := filepath.Join(cfg.SerialDir, cfg.Name+".sock")
	monitorSock := filepath.Join(cfg.MonitorDir, cfg.Name+".sock")
	qmpSock := filepath.Join(cfg.QMPDir, cfg.Name+".sock")

	args := []string{
		"-enable-kvm",
		"-cpu", "host",
		"-m", fmt.Sprintf("%d", memory),
		"-smp", fmt.Sprintf("%d", cpus),
		"-serial", fmt.Sprintf("unix:%s,server,nowait", serialSock),
		"-monitor", fmt.Sprintf("unix:%s,server,nowait", monitorSock),
		"-qmp", fmt.Sprintf("unix:%s,server,nowait", qmpSock),
		"-display", "none",
	}

	if cfg.UEFICodePath != "" && cfg.UEFIVarsPath != "" {
		args = append(args,
			"-drive", fmt.Sprintf("if=pflash,format=raw,readonly=on,file=%s", cfg.UEFICodePath),
			"-drive", fmt.Sprintf("if=pflash,format=raw,file=%s", cfg.UEFIVarsPath),
		)
	}

	hasScsi := false
	virtioBlk := 0
	for _, d := range cfg.Disks {
		if strings.HasPrefix(d.Dev, "xvd") {
			hasScsi = true
		} else {
			virtioBlk++
		}
	}
	if virtioBlk > 1 {
		return nil, fmt.Errorf("qemu: %d disques virtio-blk (vd*) déclarés, un seul est supporté — utiliser des disques xvd* pour les disques additionnels", virtioBlk)
	}
	if hasScsi {
		args = append(args, "-device", "virtio-scsi-pci,id=scsi0,bus=pci.0,addr=0x1e")
	}

	sorted := make([]DiskConfig, len(cfg.Disks))
	copy(sorted, cfg.Disks)
	// vd* avant xvd* : les disques virtio-blk bootent en premier.
	// À lettre égale de type, ordre alphabétique.
	sort.Slice(sorted, func(i, j int) bool {
		iVirtio := strings.HasPrefix(sorted[i].Dev, "vd")
		jVirtio := strings.HasPrefix(sorted[j].Dev, "vd")
		if iVirtio != jVirtio {
			return iVirtio
		}
		return sorted[i].Dev < sorted[j].Dev
	})

	for idx, d := range sorted {
		bootindex := idx + 1
		if strings.HasPrefix(d.Dev, "xvd") {
			scsiID := int(d.Dev[3] - 'a')
			args = append(args,
				"-drive", fmt.Sprintf("file=%s,if=none,id=%s", d.Path, d.Dev),
				"-device", fmt.Sprintf("scsi-hd,drive=%s,bus=scsi0.0,scsi-id=%d,bootindex=%d,serial=%s", d.Dev, scsiID, bootindex, d.Dev),
			)
		} else {
			args = append(args,
				"-drive", fmt.Sprintf("file=%s,if=none,id=%s", d.Path, d.Dev),
				"-device", fmt.Sprintf("virtio-blk-pci,drive=%s,bus=pci.0,addr=0x1f,bootindex=%d,serial=%s", d.Dev, bootindex, d.Dev),
			)
		}
	}

	// Slots 0x03 à 0x1d réservés au réseau par la carte PCI (#36). Le slot est
	// dérivé de l'index et non laissé à QEMU : c'est lui qui fixe le nom de
	// l'interface dans le guest, et un slot flottant la renomme d'un démarrage
	// à l'autre.
	if len(cfg.NICs) == 0 {
		return nil, fmt.Errorf("vm %s has no network interface", cfg.Name)
	}
	if len(cfg.NICs) > maxNICs {
		return nil, fmt.Errorf("vm %s has %d interfaces, the pci map holds %d", cfg.Name, len(cfg.NICs), maxNICs)
	}
	for i, n := range cfg.NICs {
		device := fmt.Sprintf("virtio-net-pci,netdev=net%d,mac=%s,bus=pci.0,addr=0x%02x", i, n.Mac, firstNICSlot+i)
		if n.MTU > 0 {
			device += fmt.Sprintf(",host_mtu=%d", n.MTU)
		}
		args = append(args,
			"-netdev", fmt.Sprintf("tap,id=net%d,ifname=tap%d,script=no,downscript=no", i, n.TapID),
			"-device", device,
		)
	}

	return append(args, "-daemonize"), nil
}
