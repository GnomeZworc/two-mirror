package configuration

import (
	"os"
	"path/filepath"
	"testing"
)

func writeYAML(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("impossible d'écrire le fichier de config : %v", err)
	}
	return path
}

// --- LoadConfig ---

func TestLoadConfig_ValidFile(t *testing.T) {
	path := writeYAML(t, `
database:
  path: /tmp/mydb
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig a échoué : %v", err)
	}
	if cfg.Database.Path != "/tmp/mydb" {
		t.Errorf("database.path attendu %q, obtenu %q", "/tmp/mydb", cfg.Database.Path)
	}
}

func TestLoadConfig_DefaultPath(t *testing.T) {
	// Fichier vide → viper applique la valeur par défaut
	path := writeYAML(t, "")
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig a échoué : %v", err)
	}
	if cfg.Database.Path != "/var/lib/two/data/" {
		t.Errorf("valeur par défaut attendue %q, obtenu %q", "/var/lib/two/data/", cfg.Database.Path)
	}
}

func TestLoadConfig_MissingFile_UsesDefaults(t *testing.T) {
	// Fichier inexistant : viper ignore l'erreur ReadInConfig et retourne les défauts
	cfg, err := LoadConfig("/chemin/inexistant/config.yml")
	if err != nil {
		t.Fatalf("LoadConfig devrait retourner les défauts si le fichier est absent : %v", err)
	}
	if cfg.Database.Path != "/var/lib/two/data/" {
		t.Errorf("valeur par défaut attendue, obtenu %q", cfg.Database.Path)
	}
}

func TestLoadConfig_PartialConfig_MissingDatabaseKey(t *testing.T) {
	// Fichier sans la clé database → valeur par défaut
	path := writeYAML(t, `
autrekey: valeur
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig a échoué : %v", err)
	}
	if cfg.Database.Path != "/var/lib/two/data/" {
		t.Errorf("valeur par défaut attendue, obtenu %q", cfg.Database.Path)
	}
}

func TestLoadConfig_CustomPath(t *testing.T) {
	path := writeYAML(t, `
database:
  path: /opt/two/data
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig a échoué : %v", err)
	}
	if cfg.Database.Path != "/opt/two/data" {
		t.Errorf("attendu %q, obtenu %q", "/opt/two/data", cfg.Database.Path)
	}
}

func TestLoadConfig_WatchdogDefauts(t *testing.T) {
	path := writeYAML(t, "")
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig a échoué : %v", err)
	}
	if cfg.Watchdog.Enabled {
		t.Error("watchdog.enabled devrait être false par défaut")
	}
	if cfg.Watchdog.IntervalSeconds != 60 {
		t.Errorf("watchdog.interval_seconds attendu 60, obtenu %d", cfg.Watchdog.IntervalSeconds)
	}
}

func TestLoadConfig_WatchdogValeursExplicites(t *testing.T) {
	path := writeYAML(t, `
watchdog:
  enabled: true
  interval_seconds: 30
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig a échoué : %v", err)
	}
	if !cfg.Watchdog.Enabled {
		t.Error("watchdog.enabled attendu true")
	}
	if cfg.Watchdog.IntervalSeconds != 30 {
		t.Errorf("watchdog.interval_seconds attendu 30, obtenu %d", cfg.Watchdog.IntervalSeconds)
	}
}

func TestLoadConfig_WatchdogActiveSansIntervalle(t *testing.T) {
	path := writeYAML(t, `
watchdog:
  enabled: true
`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig a échoué : %v", err)
	}
	if !cfg.Watchdog.Enabled {
		t.Error("watchdog.enabled attendu true")
	}
	if cfg.Watchdog.IntervalSeconds != 60 {
		t.Errorf("watchdog.interval_seconds attendu 60 (défaut viper), obtenu %d", cfg.Watchdog.IntervalSeconds)
	}
}

func TestLoadConfig_ExempleFourniEstValide(t *testing.T) {
	cfg, err := LoadConfig("../../../conf/agent/config.exemple.yml")
	if err != nil {
		t.Fatalf("config.exemple.yml illisible : %v", err)
	}
	if !cfg.Watchdog.Enabled {
		t.Error("config.exemple.yml devrait activer le watchdog")
	}
	if cfg.Watchdog.IntervalSeconds != 60 {
		t.Errorf("config.exemple.yml : interval_seconds attendu 60, obtenu %d", cfg.Watchdog.IntervalSeconds)
	}
	if cfg.QEMU.QMPDir == "" {
		t.Error("config.exemple.yml devrait définir qemu.qmp_dir")
	}
}
