package kv

import (
	"github.com/dgraph-io/badger/v4"
)

// ListByPrefix returns all key-value pairs whose key starts with prefix.
func ListByPrefix(db *badger.DB, prefix string) (map[string]string, error) {
	result := make(map[string]string)
	p := []byte(prefix)

	err := db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchSize = 10

		it := txn.NewIterator(opts)
		defer it.Close()

		for it.Seek(p); it.ValidForPrefix(p); it.Next() {
			item := it.Item()
			key := string(item.Key())

			val, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			result[key] = string(val)
		}
		return nil
	})

	return result, err
}
