package metadata

import (
	"git.g3e.fr/syonad/two/pkg/systemd"
	"github.com/dgraph-io/badger/v4"
)

func StartMetadata(config NoCloudConfig, db *badger.DB, dryrun bool) {
	service, _ := systemd.New()
	defer service.Close()

	LoadNcCloudInDB(config, db)
	if !dryrun {
		service.Start("metadata@" + config.Name)
	}
}

func StopMetadata(vm_name string, db *badger.DB, dryrun bool) {
	service, _ := systemd.New()
	defer service.Close()

	UnLoadNoCloudInDB(vm_name, db)
	if !dryrun {
		service.Stop("metadata@" + vm_name)
	}
}
