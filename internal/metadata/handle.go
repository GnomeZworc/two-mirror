package metadata

import (
	"fmt"

	configuration "git.g3e.fr/syonad/two/internal/config/agent"
	"git.g3e.fr/syonad/two/pkg/systemd"
)

func StartMetadata(config NoCloudConfig, cfg *configuration.Config, dryrun bool) error {
	if err := WriteNoCloudFiles(config, cfg.Metadata.RunDir); err != nil {
		return fmt.Errorf("write nocloud files for %s: %w", config.Name, err)
	}
	if dryrun {
		return nil
	}

	service, err := systemd.New()
	if err != nil {
		return fmt.Errorf("failed to connect to systemd: %w", err)
	}
	defer service.Close()

	if err := service.Start("metadata@" + config.Name + ".service"); err != nil {
		return fmt.Errorf("failed to start metadata@%s: %w", config.Name, err)
	}
	return nil
}

func StopMetadata(vmName string, cfg *configuration.Config, dryrun bool) error {
	if !dryrun {
		service, err := systemd.New()
		if err != nil {
			return fmt.Errorf("failed to connect to systemd: %w", err)
		}
		defer service.Close()

		if err := service.Stop("metadata@" + vmName + ".service"); err != nil {
			return fmt.Errorf("failed to stop metadata@%s: %w", vmName, err)
		}
	}

	return RemoveNoCloudFiles(vmName, cfg.Metadata.RunDir)
}
