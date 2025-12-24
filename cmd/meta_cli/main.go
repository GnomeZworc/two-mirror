package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
)

var DB *badger.DB

func AddInDB(dbName string, line string) error {
	// ID = partie avant le premier ';'
	id := strings.Split(line, ";")[0] + "/bash"
	key := []byte(dbName + "/" + id)

	return DB.Update(func(txn *badger.Txn) error {
		return txn.Set(key, []byte(line))
	})
}

func main() {
	conf_file := flag.String("conf", "/etc/two/agent.yml", "configuration file")
	vm_name := flag.String("vm_name", "", "Nom de la vm")
	vpc := flag.String("vpc_name", "", "vpc name")
	bind_ip := flag.String("ip", "", "bind ip")
	bind_port := flag.String("port", "", "bind port")
	ssh_key := flag.String("key", "", "Clef ssh")
	password := flag.String("pass", "", "password user")
	start := flag.Bool("start", false, "start metadata server")
	stop := flag.Bool("stop", false, "stop metadata server")

	flag.Parse()

	conf, err := configuration.LoadConfig(*conf_file)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Print(conf)

	DB = kv.InitDB(kv.Config{
		Path: conf.Database.Path,
	})
	defer DB.Close()

	fmt.Printf("conf metadata for %s\n - this key %s\n - this password %s\n", *vm_name, *ssh_key, *password)

	os.Exit(0)
}
