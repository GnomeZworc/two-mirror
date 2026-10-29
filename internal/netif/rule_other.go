//go:build !linux

package netif

import (
	"errors"
	"net"
)

func RuleAddFrom(_ net.IP, _, _ int, _ bool) error {
	return errors.New("not supported on this platform")
}

func RuleDelFromIfExists(_ net.IP, _, _ int, _ bool) error {
	return errors.New("not supported on this platform")
}
