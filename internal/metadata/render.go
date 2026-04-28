package metadata

import (
	"bytes"
	"embed"
	"os"
	"path/filepath"
	"text/template"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

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

func LoadNcCloudInDB(config NoCloudConfig, runDir string) {
	meta_data, _ := RenderConfig("templates/meta-data.tmpl", config)
	user_data, _ := RenderConfig("templates/user-data.tmpl", config)
	network_config, _ := RenderConfig("templates/network-config.tmpl", config)
	vendor_data, _ := RenderConfig("templates/vendor-data.tmpl", config)

	dir := filepath.Join(runDir, config.Name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}
	os.WriteFile(filepath.Join(dir, "meta-data"), []byte(meta_data), 0644)
	os.WriteFile(filepath.Join(dir, "user-data"), []byte(user_data), 0644)
	os.WriteFile(filepath.Join(dir, "network-config"), []byte(network_config), 0644)
	os.WriteFile(filepath.Join(dir, "vendor-data"), []byte(vendor_data), 0644)
	os.WriteFile(filepath.Join(dir, "vpc"), []byte(config.VpcName), 0644)
	os.WriteFile(filepath.Join(dir, "bind_ip"), []byte(config.BindIP), 0644)
	os.WriteFile(filepath.Join(dir, "bind_port"), []byte(config.BindPort), 0644)
}

func UnLoadNoCloudInDB(vmName string, runDir string) {
	os.RemoveAll(filepath.Join(runDir, vmName))
}
