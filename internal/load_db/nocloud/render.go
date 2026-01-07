package nocloud

import (
	"bytes"
	"embed"
	"text/template"

	"git.g3e.fr/syonad/two/pkg/db/kv"
	"github.com/dgraph-io/badger/v4"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

func renderConfig(path string, cfg Config) (string, error) {
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

func LoadNcCloudInDB(config Config, db *badger.DB) {
	meta_data, _ := renderConfig("templates/meta-data.tmpl", config)
	user_data, _ := renderConfig("templates/user-data.tmpl", config)
	network_config, _ := renderConfig("templates/network-config.tmpl", config)
	vendor_data, _ := renderConfig("templates/vendor-data.tmpl", config)

	kv.AddInDB(db, "metadata/"+config.Name+"/meta-data", meta_data)
	kv.AddInDB(db, "metadata/"+config.Name+"/user-data", user_data)
	kv.AddInDB(db, "metadata/"+config.Name+"/network-config", network_config)
	kv.AddInDB(db, "metadata/"+config.Name+"/vendor-data", vendor_data)
	kv.AddInDB(db, "metadata/"+config.Name+"/vpc", config.VpcName)
	kv.AddInDB(db, "metadata/"+config.Name+"/bind_ip", config.BindIP)
	kv.AddInDB(db, "metadata/"+config.Name+"/bind_port", config.BindPort)
}

func UnLoadNoCloudInDB(vm_name string, db *badger.DB) {
	kv.DeleteInDB(db, "metadata/"+vm_name)
}
