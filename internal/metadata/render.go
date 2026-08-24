package metadata

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

const (
	DocMetaData      = "meta-data"
	DocUserData      = "user-data"
	DocNetworkConfig = "network-config"
	DocVendorData    = "vendor-data"
)

func Documents() []string {
	return []string{DocMetaData, DocUserData, DocNetworkConfig, DocVendorData}
}

func RenderConfig(path string, cfg NoCloudConfig) (string, error) {
	tpl, err := template.ParseFS(templateFS, path)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tpl.Execute(&buf, cfg); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func renderDocument(name string, cfg NoCloudConfig) (string, error) {
	if doc, ok := cfg.Documents[name]; ok {
		return doc, nil
	}

	out, err := RenderConfig("templates/"+name+".tmpl", cfg)
	if err != nil {
		return "", fmt.Errorf("render %s: %w", name, err)
	}

	out = strings.TrimSpace(out)
	if out == "" {
		return "", nil
	}
	return out + "\n", nil
}

func WriteNoCloudFiles(config NoCloudConfig, runDir string) error {
	dir := filepath.Join(runDir, config.Name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	files := map[string]string{
		"vpc":       config.VpcName,
		"bind_ip":   config.BindIP,
		"bind_port": config.BindPort,
	}
	for _, name := range Documents() {
		doc, err := renderDocument(name, config)
		if err != nil {
			return err
		}
		files[name] = doc
	}

	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

func RemoveNoCloudFiles(vmName string, runDir string) error {
	dir := filepath.Join(runDir, vmName)
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove %s: %w", dir, err)
	}
	return nil
}
