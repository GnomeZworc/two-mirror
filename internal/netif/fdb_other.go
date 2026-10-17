//go:build !linux

package netif

import (
	"errors"
	"net"
)

func BridgeAddLocalMAC(_ string, _ net.HardwareAddr) error {
	return errors.New("not supported on this platform")
}

func BridgeHasLocalMAC(_ string, _ net.HardwareAddr) (bool, error) {
	return false, errors.New("not supported on this platform")
}
