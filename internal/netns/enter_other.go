//go:build !linux

package netns

func enter(name string) error {
	// Ignoré hors Linux
	return nil
}
