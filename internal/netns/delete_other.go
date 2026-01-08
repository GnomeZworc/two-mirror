//go:build !linux

package netns

func delete(string) error { return nil }
