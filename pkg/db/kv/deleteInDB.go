package kv

import (
	"github.com/dgraph-io/badger/v4"
)

func DeleteInDB(db *badger.DB, key string) error {
	return db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(key))
	})
}
