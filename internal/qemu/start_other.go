//go:build !linux

package qemu

import "errors"

type Config struct {
	Name, Mac, VolumePath string
	TapID, Memory, CPUs   int
	UEFICodePath          string
	UEFIVarsPath          string
	SerialDir             string
	MonitorDir            string
	QMPDir                string
}

func Start(_ Config) error {
	return errors.New("vm: not supported on this platform")
}
