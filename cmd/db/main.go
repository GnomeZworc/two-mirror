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

func CheckInDB(dbName, id string) int {
	prefix := []byte(dbName + "/")
	key := []byte(dbName + "/" + id)

	// vérifier si DB contient au moins une entrée avec ce préfixe
	hasPrefix := false

	DB.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		it.Seek(prefix)
		if it.ValidForPrefix(prefix) {
			hasPrefix = true
		}
		return nil
	})

	if !hasPrefix {
		return 1
	}

	// vérifier si la clé existe
	err := DB.View(func(txn *badger.Txn) error {
		_, err := txn.Get(key)
		return err
	})

	if err == badger.ErrKeyNotFound {
		return 2
	}

	return 0
}

func AddInDB(dbName string, line string) error {
	// ID = partie avant le premier ';'
	id := strings.Split(line, ";")[0] + "/bash"
	key := []byte(dbName + "/" + id)

	return DB.Update(func(txn *badger.Txn) error {
		return txn.Set(key, []byte(line))
	})
}

func DeleteInDB(dbName, id string) error {
	key := []byte(dbName + "/" + id + "/bash")

	return kv.DeleteInDB(DB, string(key))
}

func CountInDB(dbName, id string) int {
	prefix := []byte(dbName + "/" + id + "/bash")
	count := 0

	DB.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			count++
		}
		return nil
	})

	return count
}

func GetFromDB(dbName, id string) (string, error) {
	key := []byte(dbName + "/" + id + "/bash")

	var result string

	err := DB.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		return item.Value(func(val []byte) error {
			result = string(val)
			return nil
		})
	})

	return result, err
}

func printDB() {
	err := DB.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()

		for it.Rewind(); it.Valid(); it.Next() {
			item := it.Item()
			key := item.Key()
			err := item.Value(func(val []byte) error {
				fmt.Printf("%s:%s\n", string(key), string(val))
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		fmt.Println("Error reading DB:", err)
	}
}

func main() {
	conf_file := flag.String("conf", "/etc/two/agent.yml", "configuration file")

	flag.Parse()

	args := flag.Args()

	conf, err := configuration.LoadConfig(*conf_file)
	if err != nil {
		fmt.Println(err)
		return
	}

	DB = kv.InitDB(kv.Config{
		Path: conf.Database.Path,
	})
	defer DB.Close()

	if len(args) < 1 {
		fmt.Println("Usage: db <cmd> [args...]")
		return
	}

	cmd := args[0]

	switch cmd {
	case "check_in_db":
		if len(args) != 3 {
			fmt.Println("Usage: check_in_db <db_name> <id>")
			os.Exit(1)
		}
		ret := CheckInDB(args[1], args[2])
		os.Exit(ret)
	case "add_in_db":
		if len(args) < 3 {
			fmt.Println("Usage: add_in_db <db_name> <line...>")
			os.Exit(1)
		}
		line := strings.Join(args[2:], ";")
		if err := AddInDB(args[1], line); err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}
	case "delete_in_db":
		if len(args) != 3 {
			fmt.Println("Usage: delete_in_db <db_name> <id>")
			os.Exit(1)
		}
		if err := DeleteInDB(args[1], args[2]); err != nil {
			fmt.Println("Error:", err)
			os.Exit(1)
		}
	case "count_in_db":
		if len(args) != 3 {
			fmt.Println("Usage: count_in_db <db_name> <id>")
			os.Exit(1)
		}
		count := CountInDB(args[1], args[2])
		fmt.Println(count)
	case "get_from_db":
		if len(args) != 3 {
			fmt.Println("Usage: get_from_db <db_name> <id>")
			os.Exit(1)
		}
		line, _ := GetFromDB(args[1], args[2])
		fmt.Println(line)
	case "print":
		printDB()
		os.Exit(1)
	default:
		fmt.Println("Unknown command:", cmd)
		os.Exit(1)
	}
}
