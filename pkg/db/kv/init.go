package kv

import (
	"github.com/dgraph-io/badger/v4"
)

func InitDB() *badger.DB {
	opts := badger.DefaultOptions("./data")
	opts.Logger = nil
	opts.ValueLogFileSize = 10 << 20 // 10 Mo par fichier vlog
	opts.NumMemtables = 1
	opts.NumLevelZeroTables = 1
	opts.NumLevelZeroTablesStall = 2
	db, err := badger.Open(opts)
	if err != nil {
		panic(err)
	}
	return db
}
