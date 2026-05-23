//go:build !linux

package netif

import (
	"fmt"
	"net"
)

func GetDefaultGateway() (net.IP, error) {
	return nil, fmt.Errorf("not supported on this platform")
}
