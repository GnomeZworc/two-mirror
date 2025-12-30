package metadata

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"git.g3e.fr/syonad/two/internal/netns"
	"git.g3e.fr/syonad/two/pkg/db/kv"
)

var data NoCloudData

func getIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func getFromDB(config Config) NoCloudData {
	var netns_name string
	var port int
	var iface string

	db := kv.InitDB(kv.Config{Path: config.ConfFile})

	metadata, _ := kv.GetFromDB(db, "metadata/"+config.VmName+"/meta-data")
	userdata, _ := kv.GetFromDB(db, "metadata/"+config.VmName+"/user-data")
	networkconfig, _ := kv.GetFromDB(db, "metadata/"+config.VmName+"/network-config")
	vendordata, _ := kv.GetFromDB(db, "metadata/"+config.VmName+"/vendor-data")

	if config.Netns == "" {
		netns_name, _ = kv.GetFromDB(db, "metadata/"+config.VmName+"/vpc")
	} else {
		netns_name = config.Netns
	}

	if config.Iface == "" {
		iface, _ = kv.GetFromDB(db, "metadata/"+config.VmName+"/bind_ip")
	} else {
		iface = config.Iface
	}

	if config.Port == 0 {
		sport, _ := kv.GetFromDB(db, "metadata/"+config.VmName+"/bind_port")
		port, _ = strconv.Atoi(sport)
	} else {
		port = config.Port
	}

	return NoCloudData{
		MetaData:      metadata,
		UserData:      userdata,
		NetworkConfig: networkconfig,
		VendorData:    vendordata,
		NetNs:         netns_name,
		Iface:         iface,
		Port:          port,
	}
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	ip := getIP(r)
	path := r.URL.Path
	timestamp := time.Now().Format(time.RFC3339)
	userAgent := r.Header.Get("User-Agent")

	log.Printf("[%s] Requête IP %s vers %s | User-Agent: %s", timestamp, ip, path, userAgent)

	w.Header().Set("Content-Type", "text/yaml")

	switch path {
	case "/user-data":
		fmt.Fprint(w, data.UserData)
	case "/meta-data":
		fmt.Fprint(w, data.MetaData)
	case "/network-config":
		fmt.Fprint(w, data.NetworkConfig)
	case "/vendor-data":
		fmt.Fprint(w, data.VendorData)
	default:
		http.NotFound(w, r)
	}
}

func StartServer(config Config) {
	data = getFromDB(config)

	if data.NetNs != "" {
		if err := netns.Enter(data.NetNs); err != nil {
			log.Fatalf("Impossible d'entrer dans le netns: %v", err)
		}
	}

	http.HandleFunc("/", rootHandler)

	address := fmt.Sprintf("%s:%d", data.Iface, data.Port)
	log.Printf("Serveur NoCloud démarré sur http://%s/", address)
	log.Fatal(http.ListenAndServe(address, nil))
}
