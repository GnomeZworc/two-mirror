package netns

func Delete(name string) error {
	return delete(name)
}
