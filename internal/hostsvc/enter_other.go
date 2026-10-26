//go:build !linux

package hostsvc

func netnsExists(string) bool { return false }

func enterNetns(_ string, fn func() error) error { return fn() }
