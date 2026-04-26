//go:build !linux

package netif

import "errors"

func CreateTap(_ int, _, _ string) error {
	return errors.New("netif: tap not supported on this platform")
}
