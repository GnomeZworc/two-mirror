//go:build !linux

package netns

func call(_ string, fn func() error) error {
	return fn()
}
