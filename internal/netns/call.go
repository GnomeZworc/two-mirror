package netns

func Call(name string, fn func() error) error {
	return call(name, fn)
}
