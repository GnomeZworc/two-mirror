//go:build !linux

package netns

func call(name string, fn func() error) error {
	return fn()
}
