package netns

import (
	"os"
)

func exist(name string) bool {
	_, err := os.Stat("/var/run/netns/" + name)
	return err == nil
}

func Exist(name string) bool {
	return exist(name)
}
