package metadata

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"git.g3e.fr/syonad/two/internal/netns"
)

var data NoCloudData

func getIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

func readFile(dir, name string) string {
	b, _ := os.ReadFile(filepath.Join(dir, name))
	return strings.TrimRight(string(b), "\n")
}

func getFromFiles(config ServerConfig) NoCloudData {
	dir := filepath.Join(config.RunDir, config.VmName)

	port, _ := strconv.Atoi(readFile(dir, "bind_port"))

	return NoCloudData{
		MetaData:      readFile(dir, "meta-data"),
		UserData:      readFile(dir, "user-data"),
		NetworkConfig: readFile(dir, "network-config"),
		VendorData:    readFile(dir, "vendor-data"),
		NetNs:         readFile(dir, "vpc"),
		Iface:         readFile(dir, "bind_ip"),
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

func StartServer(config ServerConfig) {
	data = getFromFiles(config)

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
