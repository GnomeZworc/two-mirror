package metadata

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"time"

	"git.g3e.fr/syonad/two/internal/netns"
)

var data NoCloudData

var (
	iface      = flag.String("interface", "0.0.0.0", "Interface IP à écouter")
	port       = flag.Int("port", 8080, "Port à utiliser")
	file       = flag.String("file", "", "Fichier JSON contenant les données NoCloud")
	netns_name = flag.String("netns", "", "Network namespace à utiliser")
)

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

func StartServer() {
	flag.Parse()

	if *netns_name != "" {
		if err := netns.Enter(*netns_name); err != nil {
			log.Fatalf("Impossible d'entrer dans le netns: %v", err)
		}
	}

	if *file == "" {
		log.Fatal("Vous devez spécifier un fichier via --file")
	}

	raw, err := ioutil.ReadFile(*file)
	if err != nil {
		log.Fatalf("Erreur de lecture du fichier: %v", err)
	}

	if err := json.Unmarshal(raw, &data); err != nil {
		log.Fatalf("Erreur de parsing JSON: %v", err)
	}

	http.HandleFunc("/", rootHandler)

	address := fmt.Sprintf("%s:%d", *iface, *port)
	log.Printf("Serveur NoCloud démarré sur http://%s/", address)
	log.Fatal(http.ListenAndServe(address, nil))
}
