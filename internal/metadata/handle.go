package metadata

import (
	"fmt"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/pkg/systemd"
)

func StartMetadata(config NoCloudConfig, cfg *configuration.Config, dryrun bool) error {
	service, err := systemd.New()
	if err != nil {
		return fmt.Errorf("failed to connect to systemd: %w", err)
	}
	defer service.Close()

	LoadNcCloudInDB(config, cfg.Metadata.RunDir)
	if !dryrun {
		if err := service.Start("metadata@" + config.Name + ".service"); err != nil {
			return fmt.Errorf("failed to start metadata@%s: %w", config.Name, err)
		}
	}
	return nil
}

func StopMetadata(vmName string, cfg *configuration.Config, dryrun bool) error {
	service, err := systemd.New()
	if err != nil {
		return fmt.Errorf("failed to connect to systemd: %w", err)
	}
	defer service.Close()

	UnLoadNoCloudInDB(vmName, cfg.Metadata.RunDir)
	if !dryrun {
		if err := service.Stop("metadata@" + vmName + ".service"); err != nil {
			return fmt.Errorf("failed to stop metadata@%s: %w", vmName, err)
		}
	}
	return nil
}
