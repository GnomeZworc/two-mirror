//go:build !linux

package netif

import (
	"errors"
	"net"
)

func CreateTap(_ int, _, _ string, _ int, _ net.HardwareAddr) error {
	return errors.New("netif: tap not supported on this platform")
}

func TapHasMAC(_ int, _ net.HardwareAddr) (bool, error) {
	return false, errors.New("netif: tap not supported on this platform")
}

func DeleteTap(_ int, _ string) error {
	return errors.New("netif: tap not supported on this platform")
}
