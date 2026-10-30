//go:build !linux

package netif

import (
	"errors"
	"net"
)

func RouteAdd(_ string, _ *net.IPNet) error {
	return errors.New("not supported on this platform")
}

func RouteAddFrom(_ string, _ *net.IPNet, _ net.IP) error {
	return errors.New("not supported on this platform")
}

func TableRoutesVia(_ int, _ string) ([]*net.IPNet, error) {
	return nil, errors.New("route listing is only implemented on linux")
}

func RouteReplaceUnreachableDefault(_, _ int) error {
	return errors.New("not supported on this platform")
}

func RouteDelUnreachableDefaultIfExists(_, _ int) error {
	return errors.New("not supported on this platform")
}

func HasUnreachableDefault(_, _ int) (bool, error) {
	return false, errors.New("route listing is only implemented on linux")
}

func RouteDelViaIfExists(_ int, _ *net.IPNet, _ net.IP, _ string) error {
	return errors.New("not supported on this platform")
}

func RouteReplaceDev(_ int, _ *net.IPNet, _ string) error {
	return errors.New("not supported on this platform")
}
