package kv

import (
	"github.com/dgraph-io/badger/v4"
)

func GetFromDB(db *badger.DB, key string) (string, error) {
	var result string

	err := db.View(func(txn *badger.Txn) error {
		item, err := txn.Get([]byte(key))
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
