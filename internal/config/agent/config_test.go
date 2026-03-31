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
