package kv

import (
	"log"

	"github.com/dgraph-io/badger/v4"
)

func deleteKey(db *badger.DB, key string) error {
	return db.Update(func(txn *badger.Txn) error {
		return txn.Delete([]byte(key))
	})
}

func DeleteInDB(db *badger.DB, key string) error {

	prefix := []byte(key + "/")

	err := db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
			item := it.Item()
			key := item.Key()

			k := append([]byte{}, key...)
			if err := deleteKey(db, string(k)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	return deleteKey(db, key)
}
