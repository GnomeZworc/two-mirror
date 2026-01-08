//go:build !linux

package netns

func create(string) error { return nil }
