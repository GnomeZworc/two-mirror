//go:build !linux

package qemu

import "errors"

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

func Start(_ Config) error {
	return errors.New("vm: not supported on this platform")
}
