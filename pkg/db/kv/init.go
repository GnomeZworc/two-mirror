package kv

import (
	"log"

	"github.com/dgraph-io/badger/v4"
)

func InitDB(conf Config, readonly bool) *badger.DB {
	opts := badger.DefaultOptions(conf.Path).
		WithReadOnly(readonly).
		WithBypassLockGuard(readonly)
	opts.Logger = nil
	opts.ValueLogFileSize = 10 << 20 // 10 Mo par fichier vlog
	opts.NumMemtables = 1
	opts.NumLevelZeroTables = 1
	opts.NumLevelZeroTablesStall = 2
	db, err := badger.Open(opts)
	if err != nil {
		log.Fatalf("kv.InitDB (readonly=%v, path=%s): %v", readonly, conf.Path, err)
	}
	return db
}
