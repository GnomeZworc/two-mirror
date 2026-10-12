//go:build !linux

package netif

import (
	"errors"
	"net"
)

func RouteAdd(_ string, _ *net.IPNet) error {
	return errors.New("not supported on this platform")
}

func TableRoutesVia(_ int, _ string) ([]*net.IPNet, error) {
	return nil, errors.New("route listing is only implemented on linux")
}
