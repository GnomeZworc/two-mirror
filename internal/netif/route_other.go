//go:build !linux

package netif

import "net"

func RouteAdd(_ string, _ *net.IPNet) error {
	return nil
}
