package kv

import (
	"github.com/dgraph-io/badger/v4"
)

func AddInDB(db *badger.DB, key string, value string) error {
	return db.Update(func(txn *badger.Txn) error {
		return txn.Set([]byte(key), []byte(value))
	})
}
