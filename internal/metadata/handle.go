package metadata

import (
	"fmt"

	"git.g3e.fr/syonad/two/pkg/systemd"
	"github.com/dgraph-io/badger/v4"
)

func StartMetadata(config NoCloudConfig, db *badger.DB, dryrun bool) error {
	service, err := systemd.New()
	if err != nil {
		return fmt.Errorf("failed to connect to systemd: %w", err)
	}
	defer service.Close()

	LoadNcCloudInDB(config, db)
	if !dryrun {
		if err := service.Start("metadata@" + config.Name + ".service"); err != nil {
			return fmt.Errorf("failed to start metadata@%s: %w", config.Name, err)
		}
	}
	return nil
}

func StopMetadata(vm_name string, db *badger.DB, dryrun bool) error {
	service, err := systemd.New()
	if err != nil {
		return fmt.Errorf("failed to connect to systemd: %w", err)
	}
	defer service.Close()

	UnLoadNoCloudInDB(vm_name, db)
	if !dryrun {
		if err := service.Stop("metadata@" + vm_name + ".service"); err != nil {
			return fmt.Errorf("failed to stop metadata@%s: %w", vm_name, err)
		}
	}
	return nil
}
