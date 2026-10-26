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

	address := fmt.Sprintf("%s:%d", data.Iface, data.Port)
	ln, err := listen(data.NetNs, address)
	if err != nil {
		log.Fatalf("Impossible d'écouter sur %s dans le netns %q: %v", address, data.NetNs, err)
	}

	http.HandleFunc("/", rootHandler)

	log.Printf("Serveur NoCloud démarré sur http://%s/", address)
	log.Fatal(http.Serve(ln, nil))
}

func listen(netNs, address string) (net.Listener, error) {
	if netNs == "" {
		return net.Listen("tcp", address)
	}
	var ln net.Listener
	err := netns.Call(netNs, func() error {
		var err error
		ln, err = net.Listen("tcp", address)
		return err
	})
	return ln, err
}
