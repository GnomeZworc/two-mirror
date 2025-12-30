package metadata

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
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
	if config.Netns != "" {
		if err := netns.Enter(config.Netns); err != nil {
			log.Fatalf("Impossible d'entrer dans le netns: %v", err)
		}
	}

	raw, err := ioutil.ReadFile(config.File)
	if err != nil {
		log.Fatalf("Erreur de lecture du fichier: %v", err)
	}

	if err := json.Unmarshal(raw, &data); err != nil {
		log.Fatalf("Erreur de parsing JSON: %v", err)
	}

	http.HandleFunc("/", rootHandler)

	address := fmt.Sprintf("%s:%d", config.Iface, config.Port)
	log.Printf("Serveur NoCloud démarré sur http://%s/", address)
	log.Fatal(http.ListenAndServe(address, nil))
}
